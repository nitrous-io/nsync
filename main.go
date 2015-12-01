package main

import (
	"os"
	"os/signal"
	"path/filepath"

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

	if err := createPidFile(); err != nil {
		log.Fatalf("Failed to create pid file : %+v", err)
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
				if err := deletePidFile(); err != nil {
					log.Errorf("Failed to delete pidfile: %+v", err)
				}
				stopCh <- struct{}{}
				<-stopCh
				os.Exit(0)
			}
		}
	}
}

func createPidFile() error {
	var err error

	pidFilePath := filepath.Join(os.Args[2], ".nsync.pid")
	log.Infof("PidFile: %s", pidFilePath)

	if pidfileLock, err = pidfile.NewLockFile(pidFilePath, 0600); err != nil {
		return err
	}

	return nil
}

func deletePidFile() error {
	if err := pidfileLock.Remove(); err != nil {
		return err
	}

	return nil
}
