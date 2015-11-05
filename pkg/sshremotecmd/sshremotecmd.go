package sshremotecmd

import (
	"bufio"
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
}

func NewSSHRemoteCmd(sshUrl, command, sshKeyPath string, timeout time.Duration) *SSHRemoteCmd {
	return &SSHRemoteCmd{
		sshUrl:     sshUrl,
		command:    command,
		sshKeyPath: sshKeyPath,
		timeout:    timeout,
	}
}

func (s *SSHRemoteCmd) Run(outputChan chan<- string, errorChan chan<- error) error {
	session, err := s.makeSession()
	if err != nil {
		return err
	}
	defer session.Close()

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

	go func() {
		for scanner.Scan() {
			if text := scanner.Text(); text != "" {
				outputChan <- text
			}
		}

		if err := scanner.Err(); err != nil {
			errorChan <- err
		}
	}()

	if err := session.Run(s.command); err != nil {
		return err
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

	return cl.NewSession()
}
