package main

import (
	"os"
	"os/signal"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
	"github.com/nitrous-io/nsync/pkg/pidfile"
)

var pidfileLock *pidfile.LockFile

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	if len(os.Args) < 3 {
		log.Infof("usage: %s <nitrous-token> <nitrous-folder-path>", os.Args[0])
		os.Exit(1)
	}

	stopCh := make(chan struct{})
	ns := nsync.NewNsync(os.Args[1], os.Args[2], stopCh)
	go ns.StartServer()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	for {
		select {
		case s := <-c:
			log.Debugf("signal received: %s", s)

			switch s {
			case os.Interrupt, os.Kill:
				stopCh <- struct{}{}
				<-stopCh
				os.Exit(1)
			}
		}
	}
}
