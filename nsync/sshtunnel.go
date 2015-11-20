package nsync

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"golang.org/x/crypto/ssh"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

type SSHTunnel struct {
	sshHost   string
	sshConfig *ssh.ClientConfig

	stopCh      chan struct{}
	forwardings map[int]forwarding
}

type forwarding struct {
	localListener net.Listener
	remoteConn    *net.Conn
}

func (st *SSHTunnel) openPort(w http.ResponseWriter, r *http.Request) {
	port := 4000
	addr := fmt.Sprintf("localhost:%d", port)
	localListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("net.Listen failed: %v", err)
		return
	}

	st.forwardings[port] = forwarding{localListener, nil}

	go func() {
		for {
			fmt.Println("got request")
			localConn, err := localListener.Accept()
			if err != nil {
				log.Errorf("listen.Accept failed: %v", err)
				return
			}

			sshClientConn, err := ssh.Dial("tcp", st.sshHost, st.sshConfig)
			if err != nil {
				log.Errorf("ssh.Dial failed: %s", err)
				return
			}

			sshConn, err := sshClientConn.Dial("tcp", addr)
			if err != nil {
				log.Errorf("sshClientConn.Dial failed: %s", err)
				return
			}

			log.Info("sshConn", sshConn)
			log.Info("localConn", localConn)

			go func() {
				if _, err = io.Copy(sshConn, localConn); err != nil {
					log.Errorf("io.Copy failed: %v", err)
				}

				sshConn.Close()
				localConn.Close()
			}()

			go func() {
				if _, err = io.Copy(localConn, sshConn); err != nil {
					log.Errorf("io.Copy failed: %v", err)
				}
				sshConn.Close()
				localConn.Close()
			}()
		}
	}()
}

func (st *SSHTunnel) closePort(w http.ResponseWriter, r *http.Request) {
	log.Info("close port")
	port := 4000
	if forwarding, ok := st.forwardings[port]; ok {
		if err := forwarding.localListener.Close(); err != nil {
			log.Error("Failed to stop port forwarding: %+v", err)
		}
	}
}

func NewSSHTunnel(sshHost string, sshConfig *ssh.ClientConfig, stopCh chan struct{}) *SSHTunnel {
	sshTunnel := &SSHTunnel{}
	sshTunnel.sshHost = sshHost
	sshTunnel.sshConfig = sshConfig
	sshTunnel.stopCh = stopCh

	sshTunnel.forwardings = make(map[int]forwarding)
	return sshTunnel
}

// Return available port
func getPort() int {
	return 8090
}

func (st *SSHTunnel) Start() {
	log.Info("starting server")
	r := mux.NewRouter()
	r.HandleFunc("/ports", st.openPort).Methods("POST")
	r.HandleFunc("/ports/{port}", st.closePort).Methods("DELETE")

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", getPort()),
		Handler: r,
	}

	errCh := make(chan error)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Errorf("server is stopped due to: %+v", err)
			errCh <- err
		}
	}()

	select {
	case <-st.stopCh:
		log.Info("stops ssh tunneling.")
		st.stopCh <- struct{}{}
	case err := <-errCh:
		log.Errorf("got error: %+v", err)
	}
}
