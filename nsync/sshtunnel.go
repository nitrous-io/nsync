package nsync

import (
	"fmt"
	"io"
	"net"
	"sync"

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

func (st *SSHTunnel) NewStart() {
	log.Infof("Start forwarding: %+v", st.ports)

	conn, err := net.Dial("tcp", st.sshHost)
	if err != nil {
		log.Errorf("new.Dial with %s failed: %v", st.sshHost, err)
		return
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", st.sshConfig)
	if err != nil {
		log.Errorf("ssh.NewClientConn failed: %v", err)
		return
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

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
					log.Errorf("listen.Accept failed: %s", err)
					return
				}

				stream, err := sshClient.Dial("tcp", addr)
				if err != nil {
					log.Errorf("sshClientConn.Dial with %s failed: %s", addr, err)
					return
				}

				pipe(localConn, stream)
			}
		}()
	}
}

func pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) (int64, int64) {
	var sent, received int64
	var c = make(chan bool)
	var o sync.Once

	close := func() {
		src.Close()
		dst.Close()
		close(c)
	}

	go func() {
		received, _ = io.Copy(src, dst)
		o.Do(close)
	}()

	go func() {
		sent, _ = io.Copy(dst, src)
		o.Do(close)
	}()

	<-c
	return sent, received
}
