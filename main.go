package main

import (
	"os"
	"os/signal"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	if len(os.Args) < 5 {
		log.Infof("usage: %s <nitrous-token> <nitrous-folder-path> <container-slug> <remote-path> <ports:optional>", os.Args[0])
		os.Exit(1)
	}

	var ports []int
	if len(os.Args) >= 6 {
		portsInStr := strings.Split(os.Args[5], ",")
		for i := 0; i < len(portsInStr); i++ {
			port, err := strconv.Atoi(portsInStr[i])
			if err != nil {
				log.Fatalf("invalid port argument: %s", os.Args[5])
			}

			ports = append(ports, port)
		}
	}

	stopSyncCh := make(chan struct{})
	ns := nsync.NewNsync(os.Args[1], os.Args[2], os.Args[3], os.Args[4], stopSyncCh)
	go ns.Start()

	if ports != nil {
		go ns.StartSSHTennel(ports)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)

	select {
	case s := <-c:
		log.Debugf("signal received: %s", s)
		stopSyncCh <- struct{}{}
		<-stopSyncCh
	}
}
