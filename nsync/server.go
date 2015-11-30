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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nsync/nsync/event"
	filesync "github.com/nitrous-io/nsync/nsync/sync"
	"github.com/nitrous-io/nsync/nsync/tunnel"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
)

const START_PORT = 64876
const MAX_PORT = 65535

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

	messageCapacity = 256
)

type Server struct {
	token             string
	sshKeyPath        string
	pubKey            string
	nitrousFolderPath string

	stopCh           chan struct{}
	port             int
	sshTunnelClients map[string]*tunnel.SSHTunnel
	syncClients      map[string]*filesync.Sync

	message chan []byte
	event   *event.Event
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
		if !client.IsStopped() {
			client.Stop()
		}
		s.syncClients[containerSlug] = nil
	}

	// TODO: Need to make mutex lock not to allow more than 1 sync running for the same container
	syncClient := filesync.NewSync(s.token, s.sshKeyPath, s.nitrousFolderPath, containerSlug, remotePath, s.pubKey, s.event)
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
	server.event = event.NewEvent(messageCapacity)
	return server
}

func (s *Server) Start() {
	log.Info("starting server")
	r := mux.NewRouter()
	r.HandleFunc("/containers/{slug}/tunnel/start", s.startPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/tunnel/stop", s.stopPortForwarding).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/start", s.startSync).Methods("POST")
	r.HandleFunc("/containers/{slug}/sync/stop", s.stopSync).Methods("POST")
	r.HandleFunc("/subscribe", s.subscribe).Methods("GET")

	errCh := make(chan error)

	started := false
	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", s.port),
		Handler: r,
		ConnState: func(conn net.Conn, state http.ConnState) {
			if !started && state == http.StateActive {
				started = true
				log.Infof("listenting to %d", s.port)
			}
		},
	}
	go func() {
		for s.port <= MAX_PORT {
			if err := server.ListenAndServe(); err != nil {
				if err.(*net.OpError).Err.(*os.SyscallError).Err == syscall.EADDRINUSE {
					s.port = s.port + 1
					server.Addr = fmt.Sprintf("127.0.0.1:%d", s.port)
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
