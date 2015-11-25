package sshremotecmd

import (
	"bufio"
	"errors"
	"io"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/pkg/sshconfig"

	"golang.org/x/crypto/ssh"
)

var ErrRemoteCmdKilled = errors.New("remote command killed")

type SSHRemoteCmd struct {
	sshUrl     string
	command    string
	sshKeyPath string
	timeout    time.Duration

	stopCh    chan struct{} // To stop the session
	sshClient *ssh.Client
}

func NewSSHRemoteCmd(sshUrl, command, sshKeyPath string, timeout time.Duration, stopCh chan struct{}) *SSHRemoteCmd {
	return &SSHRemoteCmd{
		sshUrl:     sshUrl,
		command:    command,
		sshKeyPath: sshKeyPath,
		timeout:    timeout,
		stopCh:     stopCh,
	}
}

func (s *SSHRemoteCmd) Run(outputChan chan<- string) error {
	session, err := s.makeSession()
	if err != nil {
		return err
	}

	stdoutReader, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderrReader, err := session.StderrPipe()
	if err != nil {
		return err
	}

	outputReader := io.MultiReader(stdoutReader, stderrReader)
	scanner := bufio.NewScanner(outputReader)

	internalErrCh := make(chan error, 2)

	// listening to output stream
	go func() {
		for scanner.Scan() {
			if text := scanner.Text(); text != "" {
				outputChan <- text
			}
		}

		if err := scanner.Err(); err != nil {
			internalErrCh <- err
		}

		internalErrCh <- ErrRemoteCmdKilled
	}()

	// listening to session event
	go func() {
		if err := session.Run(s.command); err != nil {
			internalErrCh <- err
		} else {
			internalErrCh <- ErrRemoteCmdKilled
		}
	}()

	cleanup := func() {
		if err := session.Close(); err != nil {
			logrus.Errorf("Failed to close a session: %s", err)
		}
		s.closeConnection()
	}

	for {
		select {
		case <-s.stopCh:
			cleanup()
			s.stopCh <- struct{}{}
			return nil
		case err := <-internalErrCh:
			cleanup()
			return err
		}
	}

	return nil
}

func (s *SSHRemoteCmd) makeSession() (*ssh.Session, error) {
	host, config, err := sshconfig.Config(s.sshUrl, s.sshKeyPath)
	if err != nil {
		return nil, err
	}

	cl, err := sshDialTimeout("tcp", host, config, s.timeout)
	if err != nil {
		return nil, err
	}

	if s.sshClient != nil {
		s.sshClient.Close()
	}

	s.sshClient = cl
	session, err := cl.NewSession()
	if err != nil {
		return nil, err
	}

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	// Request pseudo terminal
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return nil, err
	}

	return session, nil
}

func (s *SSHRemoteCmd) closeConnection() {
	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil {
			logrus.Errorf("Failed to close ssh client: %s", err)
		}
	}
}
