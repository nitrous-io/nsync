package nsync

import (
	"fmt"
	"time"

	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	timeout = 5 * time.Second
)

func (s *Sync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error, stopChan chan struct{}) {
	watcherCommand := fmt.Sprintf("/nitrous/nitrous-watcher %s", s.remotePath)
	client := sshremotecmd.NewSSHRemoteCmd(s.sshUrl, watcherCommand, s.sshKeyPath, timeout, stopChan)
	if err := client.Run(outputChan); err != nil {
		errorChan <- err
	}
	return
}
