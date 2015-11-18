package nsync

import (
	"fmt"
	"time"

	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	timeout = 5 * time.Second
)

func (ns *Nsync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error, stopChan chan struct{}) {
	watcherCommand := fmt.Sprintf("/nitrous/nitrous-watcher %s", ns.remotePath)
	client := sshremotecmd.NewSSHRemoteCmd(ns.sshUrl, watcherCommand, ns.sshKeyPath, timeout, stopChan)
	if err := client.Run(outputChan); err != nil {
		errorChan <- err
	}
	return
}
