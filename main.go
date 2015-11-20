package main

import (
	"os"
	"os/signal"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	if len(os.Args) < 5 {
		log.Infof("usage: %s <nitrous-token> <nitrous-folder-path> <container-slug> <remote-path>", os.Args[0])
		os.Exit(1)
	}

	stopSyncCh := make(chan struct{})
	stopSSHTunnelCh := make(chan struct{})
	ns := nsync.NewNsync(os.Args[1], os.Args[2], os.Args[3], os.Args[4], stopSyncCh)
	go ns.Start()

	go ns.StartSSHTennel(stopSSHTunnelCh)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	select {
	case s := <-c:
		log.Debugf("signal received: %s", s)
		stopSyncCh <- struct{}{}
		<-stopSyncCh

		stopSSHTunnelCh <- struct{}{}
		<-stopSSHTunnelCh
	}
}
