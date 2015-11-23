package nsync

import (
	"fmt"
	"io"
	"net"

	"golang.org/x/crypto/ssh"

	log "github.com/Sirupsen/logrus"
)

type SSHTunnel struct {
	sshHost   string
	sshConfig *ssh.ClientConfig
	ports     []int
	listeners []net.Listener
}

func NewSSHTunnel(sshHost string, sshConfig *ssh.ClientConfig, ports []int) *SSHTunnel {
	sshTunnel := &SSHTunnel{}
	sshTunnel.sshHost = sshHost
	sshTunnel.sshConfig = sshConfig
	sshTunnel.ports = ports

	return sshTunnel
}

func (st *SSHTunnel) Start() {
	log.Infof("Start forwarding: %+v", st.ports)

	for i := 0; i < len(st.ports); i++ {
		port := st.ports[i]
		addr := fmt.Sprintf("localhost:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Errorf("Failed to listen to %d: %v", port, err)
			return
		}

		go func() {
			for {
				localConn, err := listener.Accept()
				if err != nil {
					log.Errorf("listen.Accept failed: %v", err)
					return
				}

				sshClientConn, err := ssh.Dial("tcp", st.sshHost, st.sshConfig)
				if err != nil {
					log.Errorf("ssh.Dial with %s failed: %s", st.sshHost, err)
					return
				}

				sshConn, err := sshClientConn.Dial("tcp", addr)
				if err != nil {
					log.Errorf("sshClientConn.Dial with %s failed: %s", addr, err)
					return
				}

				go func() {
					defer sshConn.Close()
					if _, err = io.Copy(sshConn, localConn); err != nil {
						log.Errorf("io.Copy(sshConn -> localConn) failed: %v", err)
					}
				}()

				go func() {
					defer localConn.Close()
					if _, err = io.Copy(localConn, sshConn); err != nil {
						log.Errorf("io.Copy(localConn -> sshConn) failed: %v", err)
					}
				}()
			}
		}()
	}
}
