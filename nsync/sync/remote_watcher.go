package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/nitrous-io/nsync/pkg/sshremotecmd"
)

const (
	timeout = 5 * time.Second
)

func (s *Sync) startRemoteWatcher(outputChan chan<- string, errorChan chan<- error, stopChan chan struct{}) {
	md, err := s.convertToken()
	if err != nil {
		errorChan <- err
		return
	}

	watcherCommand := fmt.Sprintf("/nitrous/nitrous-watcher %s %s", s.remotePath, md)

	// This is to kill remained process.
	// It can happen when the network connection is dropped or nsync is terminated abnormally
	killCommand := fmt.Sprintf(`ps x | grep "%s" | awk '{print $1}' | xargs kill`, watcherCommand)
	sshremotecmd.NewSSHRemoteCmd(s.sshUrl, killCommand, s.sshKeyPath, 0, stopChan).Run(nil)

	client := sshremotecmd.NewSSHRemoteCmd(s.sshUrl, watcherCommand, s.sshKeyPath, timeout, stopChan)
	if err := client.Run(outputChan); err != nil {
		errorChan <- err
	}
	return
}

func (s *Sync) convertToken() (string, error) {
	hash := sha256.New()
	if _, err := hash.Write([]byte(s.token)); err != nil {
		return "", err
	}

	md := hash.Sum(nil)
	return hex.EncodeToString(md), nil
}
