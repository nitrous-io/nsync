package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	log.Infof("Nsync pid: %d", os.Getpid())

	stopCh := make(chan struct{})
	server := nsync.NewServer(stopCh)
	go server.Start()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	s := <-c
	log.Debugf("Caught signal: %s", s)

	stopCh <- struct{}{}
	<-stopCh
	os.Exit(1)
}
