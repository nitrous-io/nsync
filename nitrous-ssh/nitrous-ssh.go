package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/nitrous-io/nsync/pkg/sshconfig"
)

const usage = `usage: nitrous-ssh -l <username> -p <port> <ssh-host> -e <escape-characters> <ssh-key-path> <unison-command>`

func main() {
	var (
		username      string
		port          int
		host          string
		escapeOptions string
		cmd           string
		sshKeyPath    string
	)

	flag.StringVar(&username, "l", "", "ssh username")
	flag.IntVar(&port, "p", 0, "ssh port")
	flag.Parse()

	if username == "" || port == 0 {
		log.Fatal(usage)
	}

	host = flag.Args()[0]
	subOptions := flag.NewFlagSet(host, flag.ExitOnError)
	subOptions.StringVar(&escapeOptions, "e", "", "escape characters")
	subOptions.Parse(flag.Args()[1:])

	optionsLen := len(subOptions.Args())
	if optionsLen < 3 {
		log.Fatal(usage)
	}

	sshKeyPath = strings.Join(subOptions.Args()[0:optionsLen-2], " ")
	cmd = strings.Join(subOptions.Args()[optionsLen-2:], " ") // /nitrous/nitrous-unison -server
	sshUrl := fmt.Sprintf("ssh://%s@%s:%d", username, host, port)

	session, err := createSession(sshUrl, sshKeyPath)
	if err != nil {
		panic(err)
	}

	defer session.Close()

	done := make(chan struct{}, 1)
	if err := copyPipes(session, done); err != nil {
		panic(err)
	}

	errCh := make(chan error)
	go func() {
		if err := session.Run(cmd); err != nil {
			errCh <- err
			return
		}
		done <- struct{}{}
	}()

	select {
	case err := <-errCh:
		var code int
		if _, err := fmt.Sscanf(err.Error(), "Process exited with: %d", &code); err == nil {
			os.Exit(code)
		}
		panic(err)
	case <-done:
		os.Exit(0)
	}
}

func createSession(sshUrl, sshKeyPath string) (*ssh.Session, error) {
	host, config, err := sshconfig.Config(sshUrl, sshKeyPath)
	if err != nil {
		log.Panic(err)
	}

	cl, err := ssh.Dial("tcp", host, config)
	if err != nil {
		log.Panic(err)
	}

	session, err := cl.NewSession()
	if err != nil {
		log.Panic(err)
	}
	return session, nil
}

func copyPipes(session *ssh.Session, done chan struct{}) error {
	var o sync.Once
	var c = make(chan bool)

	terminate := func() {
		close(c)
		time.Sleep(100 * time.Millisecond)
		done <- struct{}{}
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	go func() {
		io.Copy(stdin, os.Stdin)
		o.Do(terminate)
	}()

	go func() {
		io.Copy(os.Stdout, stdout)
		o.Do(terminate)
	}()

	go func() {
		io.Copy(os.Stderr, stderr)
		o.Do(terminate)
	}()

	return nil
}
