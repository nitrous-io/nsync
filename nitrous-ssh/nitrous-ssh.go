package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

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

	if err := copyPipes(session); err != nil {
		panic(err)
	}

	session.Run(cmd)
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

func copyPipes(session *ssh.Session) error {
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go io.Copy(stdin, os.Stdin)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdout)

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}
	go io.Copy(os.Stderr, stderr)

	return nil
}
