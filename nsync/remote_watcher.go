package nsync

import (
	"fmt"
	"time"

	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	watcherCommand = "/nitrous/nitrous-watcher /home/nitrous/code"

	timeout = 5 * time.Second
)

func (ns *Nsync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error, stopChan chan struct{}) {
	client := sshremotecmd.NewSSHRemoteCmd(ns.sshUrl, watcherCommand, ns.sshKeyPath, timeout, stopChan)
	if err := client.Run(outputChan); err != nil {
		fmt.Println("error received: ", err)
		errorChan <- err
	}
	return
}
