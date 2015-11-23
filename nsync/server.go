package nsync

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

const START_PORT = 64876
const MAX_PORT = 65535

var errNoAvailablePorts = errors.New("Could't find free port")

type Server struct {
	locks     map[string]sync.Mutex
	stopCh    chan struct{}
	pairedPid int
	pidLock   sync.Mutex
	port      int
}

func (s *Server) startSync(w http.ResponseWriter, r *http.Request) {
	containerSlug := mux.Vars(r)["slug"]
	log.Infof("start syncing for container %s", containerSlug)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "OK"}`))
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

func NewServer() *Server {
	server := &Server{}
	server.port = START_PORT
	return server
}

func (s *Server) Start() {
	log.Info("starting server")
	r := mux.NewRouter()
	r.HandleFunc("/ping", s.ping).Methods("GET")
	r.HandleFunc("/acquire", s.acquire).Methods("POST")
	r.HandleFunc("/containers/{slug}", s.startSync).Methods("POST")

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
