package tunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	log "github.com/Sirupsen/logrus"
)

var errInvalidPorts = errors.New("Invalid ports are given")

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

	sshClient, err := ssh.Dial("tcp", st.sshHost, st.sshConfig)
	if err != nil {
		log.Errorf("ssh.Conn: %v", err)
		return
	}

	for i := 0; i < len(st.ports); i++ {
		port := st.ports[i]
		addr := fmt.Sprintf("localhost:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			log.Errorf("Failed to listen to %d: %v", port, err)
			return
		}

		st.listeners = append(st.listeners, listener)

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

func (st *SSHTunnel) Stop() {
	for i := 0; i < len(st.listeners); i++ {
		st.listeners[i].Close()
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

func parsePort(str string) ([]int, error) {
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
