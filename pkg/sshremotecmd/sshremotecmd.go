package sshremotecmd

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"golang.org/x/crypto/ssh"
)

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

func (s *SSHRemoteCmd) Run(outputChan chan<- string, errorChan chan<- error) error {
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

	var internelErrCh chan error

	// listening to output stream
	go func() {
		for scanner.Scan() {
			if text := scanner.Text(); text != "" {
				outputChan <- text
			}
		}

		if err := scanner.Err(); err != nil {
			internelErrCh <- err
		}
	}()

	// listening to session event
	go func() {
		if err := session.Run(s.command); err != nil {
			internelErrCh <- err
		}
	}()

	for {
		select {
		case <-s.stopCh:
			fmt.Println("Got stop signal")
			session.Close()
			s.closeConnection()
			fmt.Println("send stop signal back")
			s.stopCh <- struct{}{}
		case err := <-internelErrCh:
			session.Close()
			s.closeConnection()
			errorChan <- err
		}
	}

	return nil
}

func (s *SSHRemoteCmd) readSSHKey() (ssh.Signer, error) {
	buf, err := ioutil.ReadFile(s.sshKeyPath)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}
	return key, err
}

func (s *SSHRemoteCmd) makeSession() (*ssh.Session, error) {
	u, err := url.Parse(s.sshUrl)
	if err != nil {
		return nil, err
	}

	key, err := s.readSSHKey()
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: u.User.Username(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(key),
		},
	}

	cl, err := sshDialTimeout("tcp", u.Host, config, s.timeout)
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
		fmt.Println("close connection")
		s.sshClient.Close()
	}
}
