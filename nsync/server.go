package nsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
)

const START_PORT = 64876
const MAX_PORT = 65535

var errNoAvailablePorts = errors.New("Could't find free port")

type Server struct {
	token             string
	sshKeyPath        string
	pubKey            string
	nitrousFolderPath string

	stopCh           chan struct{}
	locks            map[string]sync.Mutex
	pairedPid        int
	pidLock          sync.Mutex
	port             int
	sshTunnelClients map[string]*SSHTunnel
	syncClients      map[string]*Sync
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	log.Infof("ping")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) acquire(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Infof("acquire")
	s.pidLock.Lock()
	defer s.pidLock.Unlock()

	if s.pairedPid != 0 {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "current sync process is paired with %d"}`, s.pairedPid)))
		return
	}

	pid, err := strconv.Atoi(r.FormValue("pid"))
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "invalid pid: %s"}`, r.Form.Get("pid"))))
		return
	}

	s.pairedPid = pid
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startPortForwarding(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Infof("start port forwarding for container %s", containerSlug)

	sshUrl := r.FormValue("sshUrl")
	portsInStr := r.FormValue("ports")

	if portsInStr == "" || sshUrl == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "arguments are not sufficient"}`)))
		return
	}

	ports, err := parsePorts(portsInStr)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "invalid ports"}`)))
		return
	}

	if client, ok := s.sshTunnelClients[containerSlug]; ok {
		client.Stop()
	}

	host, config, err := sshconfig.Config(sshUrl, s.sshKeyPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "failed to get ssh config"}`)))
		return
	}

	s.sshTunnelClients[containerSlug] = NewSSHTunnel(host, config, ports)
	go s.sshTunnelClients[containerSlug].Start()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) stopPortForwarding(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Infof("stop port forwarding for container %s", containerSlug)

	if client, ok := s.sshTunnelClients[containerSlug]; ok {
		client.Stop()
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Infof("start syncing for container %s", containerSlug)

	sshUrl := r.FormValue("sshUrl")
	remotePath := r.FormValue("remotePath")

	if sshUrl != "" && remotePath == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "arguments are not sufficient"}`)))
		return
	}

	if client, ok := s.syncClients[containerSlug]; ok {
		client.Stop()
	}

	s.syncClients[containerSlug] = NewSync(s.token, s.sshKeyPath, s.nitrousFolderPath, containerSlug, remotePath, s.pubKey)
	go s.syncClients[containerSlug].Start()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func NewServer(token, sshKeyPath, nitrousFolderPath, pubKey string) *Server {
	server := &Server{}
	server.token = token
	server.sshKeyPath = sshKeyPath
	server.nitrousFolderPath = nitrousFolderPath
	server.pubKey = pubKey
	server.port = START_PORT
	server.sshTunnelClients = make(map[string]*SSHTunnel)
	server.syncClients = make(map[string]*Sync)
	return server
}

func (s *Server) Start() {
	log.Info("starting server")
	r := mux.NewRouter()
	r.HandleFunc("/ping", s.ping).Methods("GET")
	r.HandleFunc("/acquire", s.acquire).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/start", s.startPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/stop", s.stopPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/start", s.startSync).Methods("POST")
	// r.HandleFunc("/containers/{slug}/sync/stop", s.openPort).Methods("POST")

	errCh := make(chan error)

	go func() {
		server := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
			Handler: r,
		}

		for s.port <= MAX_PORT {
			if err := server.ListenAndServe(); err != nil {
				if err.(*net.OpError).Err.(*os.SyscallError).Err == syscall.EADDRINUSE {
					s.port = s.port + 1
					server = &http.Server{
						Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
						Handler: r,
					}
					log.Infof("port is in use. trying with %d", s.port)
					continue
				}

				log.Errorf("server is stopped due to: %+v", err)
				errCh <- err
				break
			}
		}

		if s.port > MAX_PORT {
			errCh <- errNoAvailablePorts
		}
	}()

	select {
	case <-s.stopCh:
		log.Info("stops server.")
		// Kill all child processes
		s.stopCh <- struct{}{}
	case err := <-errCh:
		log.Errorf("got error: %+v", err)
	}
}

func parsePorts(str string) ([]int, error) {
	var ports []int
	portsInStr := strings.Split(str, ",")
	for i := 0; i < len(portsInStr); i++ {
		port, err := strconv.Atoi(portsInStr[i])
		if err != nil {
			return nil, errInvalidPorts
		}

		ports = append(ports, port)
	}

	return ports, nil
}
