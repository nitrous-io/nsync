package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/nitrous-io/nsync/pkg/sshconfig"
	"github.com/nitrous-io/nsync/pkg/term"
)

const usage = `usage: nsync-ssh -l <username> -p <port> <ssh-host> -i <ssh-key-path> <remote-command>`

func main() {
	var (
		username    string
		port        int
		host        string
		escapeChars string
		sshKeyPath  string
		remoteCmd   string
		noPTY       bool
		ansiResize  bool
	)

	showUsage := func() {
		log.Fatal(usage)
	}

	flag.CommandLine.Usage = showUsage
	flag.StringVar(&username, "l", "", "ssh username")
	flag.IntVar(&port, "p", 0, "ssh port")
	flag.Parse()

	if username == "" || port == 0 {
		showUsage()
	}

	host = flag.Args()[0]
	subOptions := flag.NewFlagSet(host, flag.ExitOnError)
	subOptions.Usage = showUsage
	subOptions.StringVar(&escapeChars, "e", "", "")
	subOptions.StringVar(&sshKeyPath, "i", "", "ssh key path")
	subOptions.BoolVar(&noPTY, "T", false, "disable pty allocation")
	subOptions.BoolVar(&ansiResize, "z", false, "respond to ansi resize")
	subOptions.Parse(flag.Args()[1:])

	if sshKeyPath == "" {
		showUsage()
	}

	// unescape "/////" (*nix) "\\\\\" (windows) back to space
	sshKeyPath = strings.Replace(sshKeyPath, strings.Repeat(string(os.PathSeparator), 5), " ", -1)
	remoteCmd = strings.Join(subOptions.Args()[0:], " ")

	sshUrl := fmt.Sprintf("ssh://%s@%s:%d", username, host, port)
	session, err := createSession(sshUrl, sshKeyPath)
	if err != nil {
		log.Fatalf("Failed to create ssh session: %s", err)
	}

	defer session.Close()

	done := make(chan struct{}, 1)

	if !noPTY {
		stdin, stdout, _ /*, stderr */ := term.StdStreams()
		stdinFd, stdinIsTerminal := term.GetFdInfo(stdin)
		stdoutFd, _ := term.GetFdInfo(stdout)

		if stdinIsTerminal {
			oldState, err := term.SetRawTerminal(stdinFd)
			if err != nil {
				log.Fatalf("Failed to put stdin into raw mode: %s", err)
			}
			defer term.RestoreTerminal(stdinFd, oldState)
		}

		if err := copyPipes(session, done, ansiResize); err != nil {
			log.Fatalf("Failed to pipe stdio: %s", err)
		}

		var (
			columns = 80
			lines   = 25
		)

		if stdinIsTerminal {
			winsize, err := term.GetWinsize(stdoutFd)
			if err != nil {
				log.Fatalf("Failed to get window size: %s", err)
			}
			columns = int(winsize.Width)
			lines = int(winsize.Height)

			resizeChan := make(chan os.Signal, 16)

			if runtime.GOOS == "windows" {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()

				go func() {
					for _ = range ticker.C {
						resizeChan <- syscall.Signal(-1)
					}
					close(resizeChan)
				}()
			} else {
				signal.Notify(resizeChan, syscall.Signal(0x1c)) // SIGWINCH
				defer func() {
					signal.Stop(resizeChan)
					close(resizeChan)
				}()
			}

			go resize(resizeChan, session, stdoutFd)
		}

		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 38400,
			ssh.TTY_OP_OSPEED: 38400,
		}

		term := os.Getenv("TERM")
		if term == "" {
			term = "xterm"
		}
		if err := session.RequestPty(term, lines, columns, modes); err != nil {
			log.Fatalf("Failed to allocate pseudo terminal: %s", err)
		}

	} else {
		if err := copyPipes(session, done, false); err != nil {
			log.Fatalf("Failed to pipe stdio: %s", err)
		}
	}

	errCh := make(chan error)
	go func() {
		if remoteCmd == "" {
			remoteCmd = "$SHELL" // session.Shell() does not appear to work
		}

		if err := session.Run(remoteCmd); err != nil {
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
		log.Fatalf("Error occured: %s", err)
	case <-done:
		os.Exit(0)
	}
}

func createSession(sshUrl, sshKeyPath string) (*ssh.Session, error) {
	host, config, err := sshconfig.Config(sshUrl, sshKeyPath)
	if err != nil {
		return nil, err
	}

	cl, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, err
	}

	session, err := cl.NewSession()
	if err != nil {
		return nil, err
	}
	return session, nil
}

func copyPipes(session *ssh.Session, done chan struct{}, ansiResize bool) error {
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
		if ansiResize {
			copyWithAnsiResize(session, stdin, os.Stdin)
		} else {
			io.Copy(stdin, os.Stdin)
		}
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

func sendResize(session *ssh.Session, columns, lines int) {
	type resizeMessage struct {
		Width       uint32
		Height      uint32
		PixelWidth  uint32
		PixelHeight uint32
	}

	message := resizeMessage{
		Width:  uint32(columns),
		Height: uint32(lines),
	}

	session.SendRequest("window-change", false, ssh.Marshal(message))
}

func resize(resizeChan <-chan os.Signal, session *ssh.Session, fd uintptr) {
	var previousColumns, previousLines int

	for _ = range resizeChan {
		winsize, err := term.GetWinsize(fd)
		if err != nil {
			log.Fatalf("Failed to get window size: %s", err)
		}
		columns := int(winsize.Width)
		lines := int(winsize.Height)

		if columns == previousColumns && lines == previousLines {
			continue
		}

		sendResize(session, columns, lines)

		previousColumns = columns
		previousLines = lines
	}
}

var ansiResizePrefix = []byte("\x1b[8;")

func copyWithAnsiResize(session *ssh.Session, dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}
	buf := make([]byte, 32*1024)
	for {
		nr, er := src.Read(buf)

		iStart := bytes.Index(buf, ansiResizePrefix)
		if iStart != -1 {
			iEnd := bytes.IndexByte(buf[iStart:], byte('t'))
			if iEnd != -1 {
				iEnd += iStart + 1
				code := buf[iStart+4 : iEnd]
				var lines, columns int
				fmt.Sscanf(string(code), "%d;%dt", &lines, &columns)
				// send resize
				sendResize(session, columns, lines)

				// remove code from buf
				copy(buf[iStart:], buf[iEnd:])
				nr -= (iEnd - iStart)
			}
		}

		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}
