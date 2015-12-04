package nsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nsync/nsync/event"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
	filesync "github.com/nitrous-io/nsync/nsync/sync"
	"github.com/nitrous-io/nsync/nsync/tunnel"
	"github.com/nitrous-io/nsync/pkg/httpserver"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
)

const START_PORT = 64876
const MAX_PORT = 65535
const DEFAULT_NITROUS_HOST = "https://www.nitrous.io"

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			log.Errorf("ws error: (%d) %s", status, reason)
		},
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	errNoAvailablePorts     = errors.New("Could't find free port")
	errInvalidPortsArgument = errors.New("Given ports parameter is invalid")
	errInvalidRequest       = errors.New("token is invalid")

	messageCapacity = 256
)

type Server struct {
	token       string
	nitrousHost string
	nitrousPath string
	sshKeyPath  string
	pubKey      string
	nitrousAPI  *nitrousapi.NitrousAPI

	stopCh           chan struct{}
	port             int
	sshTunnelClients map[string]*tunnel.SSHTunnel
	syncClients      map[string]*filesync.Sync

	message chan []byte
	event   *event.Event

	router *mux.Router
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) init(w http.ResponseWriter, r *http.Request) {
	var err error

	if s.nitrousPath != "" && s.sshKeyPath != "" && s.pubKey != "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "already initialized nsync"}`))
		return
	}

	token := r.FormValue("token")
	nitrousPath := r.FormValue("path")
	nitrousHost := r.FormValue("host")

	if token == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token should be provided"}`))
		return
	}

	if nitrousPath == "" {
		var err error
		nitrousPath, err = getDefaultNitrousPath()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "failed to initalize"}`))
			return
		}
	}

	if nitrousHost == "" {
		nitrousHost = DEFAULT_NITROUS_HOST
	}

	s.token = token
	s.nitrousPath = nitrousPath
	s.nitrousHost = nitrousHost
	s.nitrousAPI = nitrousapi.NewNitrousAPI(token, nitrousHost)
	s.sshKeyPath, s.pubKey, err = InitSSHInfo(s.nitrousAPI, nitrousPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to initialize"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startPortForwarding(w http.ResponseWriter, r *http.Request) {
	if s.token == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "not initialized yet"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Start port forwarding for container %s", containerSlug)

	portsInStr := r.FormValue("ports")
	if portsInStr == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "arguments are not sufficient"}`))
		return
	}

	ports, err := parsePorts(portsInStr)
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "invalid ports"}`))
		return
	}

	_, sshUrl, err := s.nitrousAPI.GetContainerInfo(containerSlug)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to get container info"}`))
		return
	}

	if client, ok := s.sshTunnelClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.sshTunnelClients[containerSlug] = nil
	}

	host, config, err := sshconfig.Config(sshUrl, s.sshKeyPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to get ssh config"}`))
		return
	}

	s.sshTunnelClients[containerSlug] = tunnel.NewSSHTunnel(containerSlug, host, config, ports, s.event)
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
	if s.token == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "not initialized yet"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	log.Debugf("Start syncing for container %s", containerSlug)

	remotePath := r.FormValue("remotePath")
	if remotePath == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "arguments are not sufficient"}`))
		return
	}

	if client, ok := s.syncClients[containerSlug]; ok && client != nil {
		if !client.IsStopped() {
			client.Stop()
		}
		s.syncClients[containerSlug] = nil
	}

	// TODO: Need to make mutex lock not to allow more than 1 sync running for the same container
	syncClient := filesync.NewSync(s.nitrousAPI, s.token, s.sshKeyPath, s.nitrousPath, containerSlug, remotePath, s.pubKey, s.event)
	if err := syncClient.CreateLocalFolderIfAbsent(); err != nil {
		log.Errorf("Failed to create folder: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to create a folder for sync"}`))
		return
	}

	if err := syncClient.UploadSyncIgnore(); err != nil {
		log.Errorf("Failed to upload .syncignore: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to sync .sycnignore with remote"}`))
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

func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	log.Debugf("subscribe event")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Upgrade failed: %s", err)
		return
	}

	go s.writePump(conn)
}

func (s *Server) writePump(conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-s.event.MsgCh:
			log.Debugf("got message: %+v", string(message))
			if !ok {
				s.writeToConn(conn, websocket.CloseMessage, []byte{})
				return
			}
			if err := s.writeToConn(conn, websocket.TextMessage, message); err != nil {
				log.Errorf("Failed to send: %+v %+v", message, err)
				return
			}
		case <-ticker.C:
			if err := s.writeToConn(conn, websocket.PingMessage, []byte{}); err != nil {
				log.Errorf("Failed to ping: %+v", err)
				return
			}
		}
	}
}

func (s *Server) writeToConn(conn *websocket.Conn, mt int, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		log.Errorf("Failed on conn.SetWriteDeadline: %s", err)
	}
	return conn.WriteMessage(mt, payload)
}

func NewServer(stopCh chan struct{}) *Server {
	server := &Server{}
	server.stopCh = stopCh
	server.port = START_PORT
	server.sshTunnelClients = make(map[string]*tunnel.SSHTunnel)
	server.syncClients = make(map[string]*filesync.Sync)
	server.event = event.NewEvent(messageCapacity)

	r := mux.NewRouter()
	r.HandleFunc("/ping", server.ping).Methods("GET")
	r.HandleFunc("/init", server.init).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/start", server.startPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/stop", server.stopPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/start", server.startSync).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/stop", server.stopSync).Methods("POST")
	r.HandleFunc("/subscribe", server.subscribe).Methods("GET")
	r.HandleFunc("/debug/pprof/{path:.*}", pprof.Index)

	server.router = r
	return server
}

func (s *Server) Start() {
	log.Debug("starting server")

	errCh := make(chan error)
	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: s.router,
	}

	go func() {
		var (
			listener net.Listener
			err      error
		)

		for s.port <= MAX_PORT {
			listener, err = httpserver.Listen(server)
			if err != nil && err.(*net.OpError).Err.(*os.SyscallError).Err == syscall.EADDRINUSE {
				// port is taken, try another port
				server.Addr = fmt.Sprintf("127.0.0.1:%d", s.port)
				log.Infof("Port is in use. trying with %d", s.port)
				s.port += 1
				if s.port > MAX_PORT {
					errCh <- errNoAvailablePorts
					return
				}
				continue
			} else if err != nil {
				log.Errorf("failed to listen due to: %+v", err)
				errCh <- err
				return
			}
			break
		}

		log.Infof("Nsync ready at port %d", s.port)
		if err := httpserver.Serve(server, listener); err != nil {
			log.Errorf("server is stopped due to: %+v", err)
			errCh <- err
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

	if len(ports) == 0 {
		return nil, errInvalidPortsArgument
	}

	return ports, nil
}

func (s *Server) stopAll() {
	log.Infof("Stopping all exisitng sync(%d) / tunneling(%d)", len(s.syncClients), len(s.sshTunnelClients))
	for _, client := range s.syncClients {
		if client != nil && !client.IsStopped() {
			client.Stop()
		}
	}

	for _, client := range s.sshTunnelClients {
		if client != nil {
			client.Stop()
		}
	}
}

func getDefaultNitrousPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}

	return filepath.Join(u.HomeDir, "Nitrous"), nil
}
