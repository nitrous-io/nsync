package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
	"github.com/nitrous-io/nsync/pkg/pidfile"
)

var pidfileLock *pidfile.LockFile

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	log.Infof("Nsync pid: %d", os.Getpid())

	stopCh := make(chan struct{})
	ns := nsync.NewNsync(stopCh)
	go ns.StartServer()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	s := <-c
	log.Debugf("Caught signal: %s", s)

	stopCh <- struct{}{}
	<-stopCh
	os.Exit(1)
}
