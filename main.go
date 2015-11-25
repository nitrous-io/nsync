package main

import (
	"errors"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
)

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
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGUSR1)

	for {
		select {
		case s := <-c:
			log.Debugf("signal received: %s", s)

			switch s {
			case syscall.SIGUSR1:
				prof := pprof.Lookup("goroutine")
				log.Infof("=== BEGIN goroutine stack dump ====")
				prof.WriteTo(log.StandardLogger().Out, 2)
				log.Infof("=== END goroutine stack dump ===")
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
	pidFilePath := filepath.Join(os.Args[2], ".nsync.pid")

	log.Infof("PidFile: %s", pidFilePath)

	_, err := os.Stat(pidFilePath)
	if err == nil {
		return errors.New("pid file already exists.")
	}

	if !os.IsNotExist(err) {
		return err
	}

	pid := os.Getpid()
	pidBytes := []byte(strconv.Itoa(pid))
	if err = ioutil.WriteFile(pidFilePath, pidBytes, 0644); err != nil {
		return err
	}

	return nil
}

func deletePidFile() error {
	pidFilePath := filepath.Join(os.Args[2], ".nsync.pid")
	return os.Remove(pidFilePath)
}
