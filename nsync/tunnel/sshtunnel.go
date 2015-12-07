package tunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync/event"
)

var errInvalidPorts = errors.New("Invalid ports are given")

type SSHTunnel struct {
	containerSlug string
	sshHost       string
	sshConfig     *ssh.ClientConfig
	ports         []int
	event         *event.Event

	listeners []net.Listener
}

func NewSSHTunnel(containerSlug, sshHost string, sshConfig *ssh.ClientConfig, ports []int, event *event.Event) *SSHTunnel {
	sshTunnel := &SSHTunnel{}
	sshTunnel.containerSlug = containerSlug
	sshTunnel.sshHost = sshHost
	sshTunnel.sshConfig = sshConfig
	sshTunnel.ports = ports
	sshTunnel.event = event

	return sshTunnel
}

func (st *SSHTunnel) Start() error {
	log.Infof("Start forwarding: %+v", st.ports)

	sshClient, err := ssh.Dial("tcp", st.sshHost, st.sshConfig)
	if err != nil {
		log.Errorf("Failed to ssh.Dial: %v", err)
		return err
	}

	for i := 0; i < len(st.ports); i++ {
		port := st.ports[i]
		address := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", address)
		if err != nil && err.(*net.OpError).Err.(*os.SyscallError).Err == syscall.EADDRINUSE {
			st.StopWithError(err)
			return fmt.Errorf("Port %d is in use", port)
		} else if err != nil {
			log.Errorf("Failed to listen to %d: %v", port, err)
			st.StopWithError(err)
			return err
		}

		st.listeners = append(st.listeners, listener)

		go func() {
			for {
				localConn, err := listener.Accept()
				if err != nil {
					log.Errorf("Failed to listen.Accept failed: %s", err)
					st.StopWithError(err)
					return
				}

				stream, err := sshClient.Dial("tcp", address)
				if err != nil {
					log.Errorf("Failed to sshClientConn.Dial with %s failed: %s", address, err)
					st.StopWithError(err)
					return
				}

				pipe(localConn, stream)
			}
		}()
	}

	st.publishStartEvent()
	return nil
}

func (st *SSHTunnel) Stop() {
	for i := 0; i < len(st.listeners); i++ {
		st.listeners[i].Close()
	}

	st.publishStopEvent(nil)
}

func (st *SSHTunnel) StopWithError(err error) {
	for i := 0; i < len(st.listeners); i++ {
		st.listeners[i].Close()
	}

	st.publishStopEvent(err)
}

func pipe(src io.ReadWriteCloser, dst io.ReadWriteCloser) {
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

func (st *SSHTunnel) portToString() string {
	var portsStr []string
	for _, port := range st.ports {
		portStr := strconv.Itoa(port)
		portsStr = append(portsStr, portStr)
	}

	return strings.Join(portsStr, ",")
}

func (st *SSHTunnel) publishStartEvent() {
	st.event.Send("container.started-port-forwarding", map[string]string{
		"slug":  st.containerSlug,
		"ports": st.portToString(),
	})

	log.Debugf("Published container.started-port-forwarding event with %+v", st)
}

func (st *SSHTunnel) publishStopEvent(err error) {
	message := map[string]string{
		"slug":  st.containerSlug,
		"ports": st.portToString(),
	}

	if err != nil {
		message["error"] = err.Error()
	}

	st.event.Send("container.stopped-port-forwarding", message)
	log.Debugf("Published container.stopped-port-forwarding event with %+v: %s", st, err)
}
