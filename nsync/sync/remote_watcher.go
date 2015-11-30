package sync

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	timeout = 5 * time.Second
)

func (s *Sync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error, stopChan chan struct{}) {
	watcherCommand := fmt.Sprintf("/nitrous/nitrous-watcher %s %s", s.remotePath, s.token)

	// This is to kill remained process.
	// It can happen when the network connection is dropped or nsync is terminated abnormally
	killCommand := fmt.Sprintf(`ps x | grep "%s" | awk '{print $1}' | xargs kill`, watcherCommand)
	if err := sshremotecmd.NewSSHRemoteCmd(s.sshUrl, killCommand, s.sshKeyPath, 0, stopChan).Run(nil); err != nil {
		logrus.Infof("Error on killing previous command: %s", err)
	}

	client := sshremotecmd.NewSSHRemoteCmd(s.sshUrl, watcherCommand, s.sshKeyPath, timeout, stopChan)
	if err := client.Run(outputChan); err != nil {
		errorChan <- err
	}
	return
}
