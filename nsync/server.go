package nsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
	filesync "github.com/nitrous-io/nsync/nsync/sync"
	"github.com/nitrous-io/nsync/nsync/tunnel"
	"github.com/nitrous-io/nsync/pkg/httpserver"
	"github.com/nitrous-io/nsync/pkg/logger"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
	"github.com/nitrous-io/nsync/pkg/userinfo"
)

const START_PORT = 64876
const MAX_PORT = 65535
const DEFAULT_NITROUS_HOST = "https://www.nitrous.io"

var (
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

	router *mux.Router
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) init(w http.ResponseWriter, r *http.Request) {
	var err error

	if s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "already initialized nsync"}`))
		return
	}

	token := r.FormValue("token")
	nitrousPath := r.FormValue("path")
	nitrousHost := r.FormValue("host")

	if token == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	if nitrousPath == "" {
		var err error
		nitrousPath, err = getDefaultNitrousPath()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "failed to initialize"}`))
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
		logger.Errorf("failed to init ssh info: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to initialize"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startPortForwarding(w http.ResponseWriter, r *http.Request) {
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	logger.Debugf("Start port forwarding for container %s", containerSlug)

	portsInStr := r.FormValue("ports")
	if portsInStr == "" {
		logger.Errorf("Failed port forwarding for container: %s, err: port is not provided", containerSlug)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "arguments are not sufficient"}`))
		return
	}

	ports, err := tunnel.ParsePort(portsInStr)
	if err != nil {
		logger.Errorf("Failed port forwarding for container: %s, err: %s", containerSlug, err)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "invalid ports"}`))
		return
	}

	_, sshUrl, err := s.nitrousAPI.GetContainerInfo(containerSlug)
	if err != nil {
		logger.Errorf("Failed port forwarding for container: %s, err: %s", containerSlug, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to get container info"}`))
		return
	}

	if client, ok := s.sshTunnelClients[containerSlug]; ok && client != nil {
		logger.Warnf("Port forwarding was enabled. Restarting.")
		client.Stop()
		s.sshTunnelClients[containerSlug] = nil
	}

	host, config, err := sshconfig.Config(sshUrl, s.sshKeyPath)
	if err != nil {
		logger.Errorf("Failed port forwarding for container: %s, err: %s", containerSlug, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to get ssh config"}`))
		return
	}

	s.sshTunnelClients[containerSlug] = tunnel.NewSSHTunnel(containerSlug, host, config, ports)
	if err := s.sshTunnelClients[containerSlug].Start(); err != nil {
		logger.Errorf("Failed port forwarding for container: %s, err: %s", containerSlug, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to start port forwarding"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) stopPortForwarding(w http.ResponseWriter, r *http.Request) {
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	logger.Debugf("Stop port forwarding for container %s", containerSlug)

	if client, ok := s.sshTunnelClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.sshTunnelClients[containerSlug] = nil
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	logger.Debugf("Start syncing for container %s", containerSlug)

	remotePath := r.FormValue("remote_path")
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
	syncClient, err := filesync.NewSync(s.nitrousAPI, s.token, s.sshKeyPath, s.nitrousPath, containerSlug, remotePath, s.pubKey)
	if err != nil {
		logger.Errorf("Failed to start sync with container(%s): %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to start sync"}`))
		return
	}

	if err := syncClient.CreateLocalFolderIfAbsent(); err != nil {
		logger.Errorf("Failed to create folder: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to create a folder for sync"}`))
		return
	}

	if err := syncClient.UploadSyncIgnore(); err != nil {
		logger.Errorf("Failed to upload .syncignore: %s", err)
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
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	logger.Debugf("Stop syncing for container %s", containerSlug)

	if client, ok := s.syncClients[containerSlug]; ok && client != nil {
		client.Stop()
		s.syncClients[containerSlug] = nil
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	path := r.FormValue("path")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "path is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	client, ok := s.syncClients[containerSlug]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "sync is not running"}`))
		return
	}

	if err := client.UploadFile(path); err != nil {
		logger.Errorf("Failed to upload file: %s, err: %v", path, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to upload file"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) registerSSHKey(w http.ResponseWriter, r *http.Request) {
	if !s.isInitialized() {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "token is required"}`))
		return
	}

	containerSlug := mux.Vars(r)["slug"]
	if err := s.nitrousAPI.UploadPublicKey(containerSlug, s.pubKey); err != nil {
		logger.Errorf("Failed to register ssh key: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "failed to register ssh key"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	logLevel := r.FormValue("log_level")
	if logLevel == "" {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "arguments are not sufficient"}`))
		return
	}

	if !logger.IsValidLogLevel(logLevel) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "invalid log level value"}`))
		return
	}

	logger.SetLogLevel(logLevel)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
}

func NewServer(stopCh chan struct{}) *Server {
	server := &Server{}
	server.stopCh = stopCh
	server.port = START_PORT
	server.sshTunnelClients = make(map[string]*tunnel.SSHTunnel)
	server.syncClients = make(map[string]*filesync.Sync)

	r := mux.NewRouter()
	r.HandleFunc("/ping", server.ping).Methods("GET")
	r.HandleFunc("/init", server.init).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/start", server.startPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/stop", server.stopPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/start", server.startSync).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/stop", server.stopSync).Methods("POST")
	r.HandleFunc("/containers/{slug}/upload", server.uploadFile).Methods("POST")
	r.HandleFunc("/containers/{slug}/register_ssh_key", server.registerSSHKey).Methods("POST")
	r.HandleFunc("/settings", server.updateSettings).Methods("PUT")
	r.HandleFunc("/debug/pprof/{path:.*}", pprof.Index)

	server.router = r
	return server
}

func (s *Server) Start() {
	logger.Debug("starting server")

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
			if err != nil && (err.(*net.OpError).Err.(*os.SyscallError).Err == syscall.EADDRINUSE ||
				// For windows, refer to https://msdn.microsoft.com/en-us/library/windows/desktop/ms740668#WSAEADDRINUSE
				strings.Contains(err.Error(), "Only one usage of each socket address (protocol/network address/port) is normally permitted.")) {
				// port is taken, try another port
				s.port += 1
				server.Addr = fmt.Sprintf("127.0.0.1:%d", s.port)
				logger.Infof("Port is in use. trying with %d", s.port)

				if s.port > MAX_PORT {
					errCh <- errNoAvailablePorts
					return
				}
				continue
			} else if err != nil {
				logger.Errorf("failed to listen due to: %+v", err)
				errCh <- err
				return
			}
			break
		}

		logger.Event("listening", s.port)
		if err := httpserver.Serve(server, listener); err != nil {
			logger.Errorf("server is stopped due to: %+v", err)
			errCh <- err
		}
	}()

	select {
	case <-s.stopCh:
		logger.Info("stopping server....")
		s.stopAll()
		s.stopCh <- struct{}{}
	case err := <-errCh:
		logger.Errorf("server is stopped with error: %+v", err)
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
	logger.Infof("Stopping all exisitng sync(%d) / tunneling(%d)", len(s.syncClients), len(s.sshTunnelClients))
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

func (s *Server) isInitialized() bool {
	return s.token != "" && s.nitrousPath != "" && s.sshKeyPath != "" && s.pubKey != ""
}

func getDefaultNitrousPath() (string, error) {
	home, err := userinfo.CurrentHomeFolder()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, "Nitrous"), nil
}
