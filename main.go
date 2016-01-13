package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/nitrous-io/nsync/nsync"
	"github.com/nitrous-io/nsync/pkg/logger"
)

var version string

func main() {
	if os.Getenv("NSYNC_LOG_LEVEL") != "" {
		logger.SetLogLevel(os.Getenv("NSYNC_LOG_LEVEL"))
	}

	nVersionFlag := flag.Bool("v", false, "Display the version and exit")
	flag.Parse()

	if *nVersionFlag {
		fmt.Printf("nsync version %v\n", version)
		os.Exit(0)
	}

	go logger.Start()
	logger.Event("pid", os.Getpid())

	stopCh := make(chan struct{})
	server := nsync.NewServer(stopCh)
	go server.Start()

	interruptChan := make(chan struct{})

	go catchSignal(interruptChan)
	go catchStdinClosure(interruptChan)

	stopCh <- (<-interruptChan)
	<-stopCh
	os.Exit(1)
}

func catchSignal(c chan<- struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	s := <-sigChan

	logger.Debugf("Caught signal: %s", s)
	c <- struct{}{}
}

func catchStdinClosure(c chan<- struct{}) {
	_, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		logger.Fatal(err)
	}
	// no error means EOF read
	c <- struct{}{}
}
