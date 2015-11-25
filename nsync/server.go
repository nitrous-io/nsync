package nsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	filesync "github.com/nitrous-io/nsync/nsync/sync"
	"github.com/nitrous-io/nsync/nsync/tunnel"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
)

const START_PORT = 64876
const MAX_PORT = 65535

var errNoAvailablePorts = errors.New("Could't find free port")
var errInvalidPortsArgument = errors.New("Given ports parameter is invalid")

type Server struct {
	token             string
	sshKeyPath        string
	pubKey            string
	nitrousFolderPath string

	stopCh           chan struct{}
	port             int
	sshTunnelClients map[string]*tunnel.SSHTunnel
	syncClients      map[string]*filesync.Sync
}

func (s *Server) startPortForwarding(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Start port forwarding for container %s", containerSlug)

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

	if client, ok := s.sshTunnelClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.sshTunnelClients[containerSlug] = nil
	}

	host, config, err := sshconfig.Config(sshUrl, s.sshKeyPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "failed to get ssh config"}`)))
		return
	}

	s.sshTunnelClients[containerSlug] = tunnel.NewSSHTunnel(host, config, ports)
	go s.sshTunnelClients[containerSlug].Start()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) stopPortForwarding(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Stop port forwarding for container %s", containerSlug)

	if client, ok := s.sshTunnelClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.sshTunnelClients[containerSlug] = nil
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Start syncing for container %s", containerSlug)

	sshUrl := r.FormValue("sshUrl")
	remotePath := r.FormValue("remotePath")

	if sshUrl != "" && remotePath == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(fmt.Sprintf(`{"error": "arguments are not sufficient"}`)))
		return
	}

	if client, ok := s.syncClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.syncClients[containerSlug] = nil
	}

	// Need to make mutex lock not to allow more than 1 sync running for the same container
	syncClient := filesync.NewSync(s.token, s.sshKeyPath, s.nitrousFolderPath, containerSlug, remotePath, s.pubKey)
	if err := syncClient.CreateLocalFolderIfAbsent(); err != nil {
		log.Errorf("Failed to create folder: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "failed to create a folder for sync"}`)))
		return
	}

	if err := syncClient.UploadSyncIgnore(); err != nil {
		log.Errorf("Failed to upload .syncignore: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"error": "failed to sync .sycnignore with remote"}`)))
		return
	}

	s.syncClients[containerSlug] = syncClient
	go s.syncClients[containerSlug].Start()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) stopSync(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Stop syncing for container %s", containerSlug)

	if client, ok := s.syncClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.syncClients[containerSlug] = nil
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func NewServer(token, sshKeyPath, nitrousFolderPath, pubKey string, stopCh chan struct{}) *Server {
	server := &Server{}
	server.token = token
	server.sshKeyPath = sshKeyPath
	server.nitrousFolderPath = nitrousFolderPath
	server.pubKey = pubKey
	server.stopCh = stopCh
	server.port = START_PORT
	server.sshTunnelClients = make(map[string]*tunnel.SSHTunnel)
	server.syncClients = make(map[string]*filesync.Sync)
	return server
}

func (s *Server) Start() {
	log.Info("starting server")
	r := mux.NewRouter()
	r.HandleFunc("/containers/{slug}/tunnel/start", s.startPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/stop", s.stopPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/start", s.startSync).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/stop", s.stopSync).Methods("POST")

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
		log.Info("stopping server....")
		s.stopAll()
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
			return nil, errInvalidPortsArgument
		}

		ports = append(ports, port)
	}

	return ports, nil
}

func (s *Server) stopAll() {
	log.Infof("Stopping all exisitng sync(%d) / tunneling(%d)", len(s.syncClients), len(s.sshTunnelClients))
	for _, client := range s.syncClients {
		if client != nil {
			client.Stop()
		}
	}

	for _, client := range s.sshTunnelClients {
		if client != nil {
			client.Stop()
		}
	}
}
