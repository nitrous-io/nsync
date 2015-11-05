package nsync

import (
	"time"

	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	watcherCommand = "/nitrous/nitrous-watcher /home/nitrous/code"

	timeout = 5 * time.Second
)

func (ns *Nsync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error) {
	client := sshremotecmd.NewSSHRemoteCmd(ns.sshUrl, watcherCommand, ns.sshKeyPath, timeout)
	if err := client.Run(outputChan, errorChan); err != nil {
		errorChan <- err
	}
	return
}
