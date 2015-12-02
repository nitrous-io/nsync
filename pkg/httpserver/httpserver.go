package httpserver

import (
	"net"
	"net/http"
	"time"
)

// Taken from net/http/server.go
// The purpose of this code is to split ListeAndServe into two functions
// `Listen` and `Serve`.

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)
	return tc, nil
}

func Listen(srv *http.Server) (net.Listener, error) {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	return net.Listen("tcp", addr)
}

func Serve(srv *http.Server, ln net.Listener) error {
	return srv.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
}
