package sync

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kardianos/osext"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
	"github.com/nitrous-io/nsync/pkg/envmap"
	"github.com/nitrous-io/nsync/pkg/logger"
	"github.com/nitrous-io/nsync/pkg/pathmatch"
	"github.com/nitrous-io/nsync/pkg/shortname"
	"github.com/nitrous-io/nsync/pkg/sshconfig"
	"github.com/nitrous-io/nsync/pkg/syncignore"
	"github.com/pkg/sftp"
	"github.com/rjeczalik/notify"
)

var (
	exeFolderPath = mustString(osext.ExecutableFolder())
	isWindows     = runtime.GOOS == "windows"

	unisonExePath string
	sshExePath    string

	ignoreList     = []string{".unison", ".git", ".svn", ".hg", "CVS", ".DS_Store", ".AppleDouble", ".LSOverride", ".Spotlight-V100", ".Trashes", "Thumbs.db", "desktop.ini", "*.lock", "*.sock", "*.pid", "*.sw?", ".unison*", "._*", "*~"}
	ignorePathList = []string{"tmp/sessions", "tmp/cache", "tmp/pids", "tmp/sockets"}

	remoteChangeRe = regexp.MustCompile(`^\s*Change\([^\)]+\):\s*(.*?)\s*$`)

	syncIgnoreFile = ".syncignore"
	tempFolderPath = "/tmp"
)

const fileSyncDelay = 100 * time.Millisecond
const maxRemoteWatcherRetry = 5
const retryInterval = 5 * time.Second
const remoteWatcherTerminateTimeout = 3 * time.Second
const unisonTerminateTimeout = 3 * time.Second

func init() {
	createSymlinkIfNotExist := func() {
		sshExeRealPath := filepath.Join(exeFolderPath, "nsync-ssh")
		sshExeSymlinkPath := filepath.Join(tempFolderPath, "nsync-ssh")
		if _, err := os.Lstat(sshExeSymlinkPath); err == nil {
			if err := os.Remove(sshExeSymlinkPath); err != nil {
				logger.Panicf("Failed to remote existing symlink: %+v", err)
			}
		} else if err != nil && !os.IsNotExist(err) {
			logger.Panicf("Failed to get stats of nsync-ssh symlink: %+v", err)
		}

		if err := os.Symlink(sshExeRealPath, sshExeSymlinkPath); err != nil {
			logger.Panicf("Failed to create symlink of nsync-ssh: %+v", err)
		}
		sshExePath = sshExeSymlinkPath
	}

	if runtime.GOOS == "darwin" {
		unisonExePath = filepath.Join(exeFolderPath, "nitrous-unison")
		createSymlinkIfNotExist()
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		unisonExePath = filepath.Join(exeFolderPath, "nitrous-unison")
		sshExePath = filepath.Join(exeFolderPath, "nitrous-unison")
		createSymlinkIfNotExist()
	} else if runtime.GOOS == "windows" {
		var err error
		unisonExePath, err = shortname.Get(filepath.Join(exeFolderPath, "nitrous-unison.exe"))
		if err != nil {
			logger.Panicf("Failed to genereate unison exec path: %+v", err)
		}
		sshExePath, err = shortname.Get(filepath.Join(exeFolderPath, "nsync-ssh.exe"))
		if err != nil {
			logger.Panicf("Failed to genereate ssh exec path: %+v", err)
		}
	} else {
		logger.Panicf("Unsupported platform: %s-%s!", runtime.GOOS, runtime.GOARCH)
	}

	for i, p := range ignorePathList {
		ignorePathList[i] = filepath.Join(strings.Split(p, "/")...)
	}
}

func mustString(s string, err error) string {
	if err != nil {
		logger.Panic(err)
	}
	return s
}

type Sync struct {
	nitrousAPI *nitrousapi.NitrousAPI

	token             string
	nitrousFolderPath string
	containerSlug     string
	remotePath        string
	sshKeyPath        string
	localPath         string
	pubKey            string

	sshUrl   string
	hostSlug string

	dirty       bool
	syncingLock sync.Mutex
	syncing     bool

	shouldIgnoreArchives bool
	shouldIgnoreLocks    bool

	lastError               error
	lastErrorDetails        string
	errorIsFatal            bool
	retryDelay              time.Duration
	remoteWatcherRetryCount int

	lastTransferCount int

	syncIgnoreList []string

	state   string
	errorCh chan error

	stopCh       chan struct{}
	stopRemoteCh chan struct{}
	stopUnisonCh chan struct{}

	lock *sync.Mutex
}

func NewSync(nitrousAPI *nitrousapi.NitrousAPI, token, sshKeyPath, nitrousFolderPath, containerSlug, remotePath, pubKey string) (*Sync, error) {
	s := &Sync{
		nitrousAPI:        nitrousAPI,
		token:             token,
		sshKeyPath:        filepath.Clean(sshKeyPath),
		nitrousFolderPath: filepath.Clean(nitrousFolderPath),
		containerSlug:     containerSlug,
		remotePath:        remotePath,
		pubKey:            pubKey,
		state:             "created",
		lock:              &sync.Mutex{},
	}

	if err := nitrousAPI.UploadPublicKey(s.containerSlug, s.pubKey); err != nil {
		logger.Errorf("sync: Failed upload ssh key: %+v ", err)
		return nil, err
	}

	hostId, sshUrl, err := nitrousAPI.GetContainerInfo(s.containerSlug)
	if err != nil {
		logger.Errorf("Failed to get container info: %+v", err)
		return nil, err
	}

	logger.Debug("host id obtained: ", hostId)
	logger.Debug("SSH URL obtained: ", sshUrl)
	s.sshUrl = sshUrl

	hostSlug, _, err := nitrousAPI.GetHostInfo(hostId)
	if err != nil {
		logger.Panicf("Failed to get host info: %+v", err)
	}
	logger.Debug("host slug obtained: ", hostSlug)

	s.hostSlug = hostSlug
	s.localPath = filepath.Join(s.nitrousFolderPath, s.hostSlug, s.containerSlug)

	s.remoteWatcherRetryCount = 0

	return s, nil
}

func (s *Sync) Start() {
	s.lock.Lock()
	s.setState("started")
	s.publishMessage("container.sync-enabled", nil)
	s.lock.Unlock()

	runSync := func() {
		s.syncingLock.Lock()
		if s.syncing {
			s.dirty = true
		} else {
			s.syncing = true
			go func() {
				s.publishMessage("container.sync-started", nil)
				s.performSync()
				s.syncingLock.Lock()
				s.syncing = false
				s.syncingLock.Unlock()
				s.publishMessage("container.sync-finished", nil)
			}()
		}
		s.syncingLock.Unlock()
	}

	s.publishMessage("container.sync-started", nil)
	s.performSync()
	s.publishMessage("container.sync-finished", nil)

	if s.lastError != nil && s.errorIsFatal {
		logger.Errorf("Local watcher is terminated: %+v", s.lastError)
		s.publishMessage("container.sync-disabled", s.lastError)
		return
	}

	// When stopped while first syncing
	if s.IsStopped() {
		s.publishMessage("container.sync-disabled", nil)
		return
	}

	notifyChan := make(chan notify.EventInfo, 1000)
	remoteChan := make(chan string, 1000)
	errorChan := make(chan error, 1)

	var timer *time.Timer
	handleChange := func(changedPath string, local bool) {
		if !pathmatch.PathMatchAny(changedPath, append(ignoreList, ignorePathList...)...) {
			logger.Debug("changed path ")
			if local {
				logger.Debug("(local): ")
			} else {
				logger.Debug("(remote): ")
			}
			logger.Debug(changedPath)

			if timer != nil {
				timer.Reset(fileSyncDelay)
			} else {
				timer = time.AfterFunc(fileSyncDelay, runSync)
			}
		}
	}

	logger.Debugf("Watching %s/%s...", s.sshUrl, s.remotePath)
	s.stopRemoteCh = make(chan struct{})
	go s.startRemoteWatcher(remoteChan, errorChan, s.stopRemoteCh)

	logger.Debugf("Watching %s...", s.localPath)
	if err := notify.Watch(filepath.Join(s.localPath, "..."), notifyChan, notify.All); err != nil {
		s.errorCh <- err
	}
	defer notify.Stop(notifyChan)

	s.errorCh = make(chan error)
	s.stopCh = make(chan struct{})

	for {
		select {
		case change := <-notifyChan: // local file system change
			{
				handleChange(change.Path(), true)
			}
		case err := <-s.errorCh: // local file sync error
			{
				logger.Errorf("Local watcher is terminated: %+v", err)
				s.stopDueToError(err)
				return
			}
		case change := <-remoteChan: // remote file system change
			{
				if s.remoteWatcherRetryCount != 0 {
					logger.Debugf("Remote watcher remote count is reset with %s", change)
					s.remoteWatcherRetryCount = 0 // reset the counter
				}

				match := remoteChangeRe.FindStringSubmatch(change)
				if len(match) >= 2 {
					handleChange(match[1], false)
				}
			}
		case err := <-errorChan: // remote watcher error
			{
				logger.Errorf("Got error message from remote watcher: %+v", err)
				if s.remoteWatcherRetryCount < maxRemoteWatcherRetry && !s.IsStopped() {
					s.remoteWatcherRetryCount++
					time.Sleep(retryInterval)
					go s.startRemoteWatcher(remoteChan, errorChan, s.stopRemoteCh)
					logger.Infof("Retrying to connect to server: %d", s.remoteWatcherRetryCount)
				} else {
					logger.Info("It reached limit of retrying. Stopping sync")
					s.stopDueToError(err)
					return
				}
			}
		case <-s.stopCh: // stop event from main process
			{
				logger.Infof("s.stopCh got message")
				s.setState("stopped")
				return
			}
		}
	}
}

func (s *Sync) CreateLocalFolderIfAbsent() error {
	if _, err := os.Stat(s.localPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(s.localPath, 0700); err != nil {
				return err
			}
		} else {
			return err
		}

		s.shouldIgnoreArchives = true
	}

	return nil
}

func (s *Sync) performSync() {
	logger.Debug("sync: Begin...")

	err := s.parseSyncIgnore()
	if err != nil {
		// Only returns error since we can still sync with previous value or empty value.
		logger.Warnf("sync: Failed to parse .syncignore: %+v", err)
	}

	s.lastError = nil
	s.lastErrorDetails = ""
	s.errorIsFatal = false
	s.retryDelay = 10 * time.Second
	s.dirty = false
	s.lastTransferCount = -1

	cmd := s.unisonCmd()
	outputCh := make(chan string)
	errCh := make(chan error)
	if s.stopUnisonCh != nil {
		close(s.stopUnisonCh)
	}
	s.stopUnisonCh = make(chan struct{})

	if err := s.runCommand(cmd, outputCh, errCh); err != nil {
		s.lastError = err
	} else {
		select {
		case outStr := <-outputCh:
			s.handleOutput(outStr)
		case err := <-errCh:
			logger.Errorf("sync: Failed to run unison: %s", err)
			s.lastError = err
		case <-s.stopUnisonCh:
			logger.Infof("sync: Killing unison")
			cmd.Process.Kill()
			s.stopUnisonCh <- struct{}{}
			return
		}
	}

	if s.lastError != nil {
		logger.Errorf("sync: Error occurred: %v", s.lastError)

		if s.lastError == ErrSSHAccessDenied {
			if err := s.nitrousAPI.UploadPublicKey(s.containerSlug, s.pubKey); err != nil {
				logger.Panicf("sync: Failed upload ssh key: %+v ", err)
			}
		}

		// It fails to run unison command. So no way to handle output.
		// This happens when sshcmd or sshargs are not valid.
		if strings.Contains(s.lastError.Error(), "exit status 3") {
			s.errorIsFatal = true
		}

		if s.errorIsFatal {
			logger.Errorf("sync: error is fatal: %s", s.lastError)
			// For the first sync, it does not wait for the channel.
			// So it could hang here. Make the error channl nil,
			// so that it does not hang for any fatal errors on first sync
			if s.errorCh != nil {
				s.errorCh <- s.lastError
			}
			return
		}

		time.AfterFunc(s.retryDelay, func() {
			if s.IsStopped() {
				return
			}

			logger.Info("sync: Retrying...")
			s.performSync()
		})
	}

	if s.lastTransferCount != -1 {
		logger.Infof("sync: %d files transferred.", s.lastTransferCount)
	}

	logger.Debug("Sync end!")

	if s.dirty {
		logger.Debug("changes happened while syncing, syncing again...")
		s.performSync()
	}
}

func (s *Sync) unisonCmd() *exec.Cmd {
	sshRemotePath := fmt.Sprintf("%s/%s", s.sshUrl, s.remotePath)

	// nsync-ssh treats "/////" (*nix) "\\\\\" (windows) as space
	// since sshKeyPath is already filepath.Clean-ed, deliberate use of
	// repeated path separators should already be gone.
	sshKeyPathEscaped := strings.Replace(s.sshKeyPath, " ", strings.Repeat(string(os.PathSeparator), 5), -1)

	sshArgs := []string{
		"-i",
		sshKeyPathEscaped,
		"-T",
	}

	args := []string{
		s.localPath,              // path to watch
		sshRemotePath,            // ssh remote path
		"-prefer", sshRemotePath, // prefer server's copy when there is a conflict
		"-ui=text",             // CLI mode
		"-auto",                // automatically accept default actions
		"-batch",               // don't ask questions
		"-perms=777",           // synchronize all permissions
		"-confirmbigdel=false", // don't ask about whole-replica deletes
		"-confirmmerge=false",  // don't ask before committing a merge
		"-log=false",           // don't log to file
		"-rsrc=false",          // don't sync resource forks
		"-sshcmd", sshExePath,  // local ssh command path
		"-sshargs", strings.Join(sshArgs, " "), // arguments to ssh command
		"-servercmd=/nitrous/nitrous-unison", // location of unison binary on server
		"-times", // sync modified times
		"-ignore", fmt.Sprintf("Name {%s}", strings.Join(ignoreList, ",")),
		"-ignore", fmt.Sprintf("Path {%s}", strings.Join(ignorePathList, ",")),
		"-ignorecase=true",
	}

	if _, err := os.Stat(s.localPath); os.IsNotExist(err) || s.shouldIgnoreArchives {
		s.shouldIgnoreArchives = false
		args = append(args, "-ignorearchives")
	}

	if s.shouldIgnoreLocks {
		s.shouldIgnoreLocks = false
		args = append(args, "-ignorelocks")
	}

	if len(s.syncIgnoreList) > 0 {
		for i := 0; i < len(s.syncIgnoreList); i++ {
			args = append(args, "-ignore", s.syncIgnoreList[i])
		}
	}

	logger.Debugf("%s %s", unisonExePath, strings.Join(args, " "))

	cmd := exec.Command(unisonExePath, args...)

	env := envmap.EnvToMap(os.Environ())
	env["UNISON"] = filepath.Join(s.localPath, ".unison")

	cmd.Env = append(cmd.Env, envmap.MapToEnv(env)...)
	cmd.Dir = exeFolderPath

	return cmd
}

func (s *Sync) runCommand(cmd *exec.Cmd, outputCh chan<- string, errCh chan<- error) error {
	stdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		logger.Errorf("sync: Failed to connect to stdout of unison command: %s", err)
		return err
	}

	stderrReader, err := cmd.StderrPipe()
	if err != nil {
		logger.Errorf("sync: Failed to connect to stderr of unison command: %s", err)
		return err
	}

	// This is to avoid "0: bad file descriptor".
	// Start scanning output after the process is started.
	if err := cmd.Start(); err != nil {
		logger.Errorf("sync: Failed to start unison command: %s", err)
		return err
	}

	go func() {
		var outputBuffer bytes.Buffer
		logger.Debug("sync: Output from unison: ")

		scanner := bufio.NewScanner(io.MultiReader(stdoutReader, stderrReader))

		for scanner.Scan() {
			if b := scanner.Bytes(); len(b) > 0 {
				logger.Debug(string(b))
				outputBuffer.Write(b)
			}
		}

		// It happens when stdout or stderr is closed while waiting.
		// Usually happens when there is no changes.
		if err := scanner.Err(); err != nil {
			logger.Warnf("sync: Failure on scanner. output: %s err: %s", outputBuffer.String(), err)
		}

		outputCh <- outputBuffer.String()
	}()

	go func() {
		if err := cmd.Wait(); err != nil {
			errCh <- err
		}
	}()

	return nil
}

func (s *Sync) UploadSyncIgnore() error {
	// Copy .syncignore from remote
	host, config, err := sshconfig.Config(s.sshUrl, s.sshKeyPath)
	if err != nil {
		return err
	}

	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return err
	}
	defer conn.Close()

	sftp, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}
	defer sftp.Close()

	syncIgnoreLocalPath := filepath.Join(s.localPath, syncIgnoreFile)
	syncIgnoreRemotePath := filepath.Join(s.remotePath, syncIgnoreFile)

	var localModTime, remoteModTime time.Time

	fi, err := os.Stat(syncIgnoreLocalPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if fi != nil && !os.IsNotExist(err) {
		localModTime = fi.ModTime()
	}

	fi, err = sftp.Stat(syncIgnoreRemotePath)
	if err != nil {
		if strings.Contains(err.Error(), "SSH_FX_NO_SUCH_FILE") {
			return nil
		}
		return err
	}

	remoteModTime = fi.ModTime()

	if localModTime.Before(remoteModTime) {
		f, err := sftp.OpenFile(syncIgnoreRemotePath, os.O_RDONLY)
		if err != nil {
			return err
		}
		defer f.Close()

		logger.Debug("Downloading .syncignore...")
		content, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(syncIgnoreLocalPath, content, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Sync) parseSyncIgnore() error {
	syncIgnorePath := filepath.Join(s.localPath, syncIgnoreFile)
	if _, err := os.Stat(syncIgnorePath); err == nil {
		ignoreContent, err := ioutil.ReadFile(syncIgnorePath)
		if err != nil {
			return err
		}
		syncIgnoreList := syncignore.Parse(string(ignoreContent))
		if err != nil {
			return err
		}
		s.syncIgnoreList = append(s.syncIgnoreList, syncIgnoreList...)
	} else if err != nil && os.IsNotExist(err) {
		logger.Infof(".syncignore not found: %+v", err)
	} else {
		logger.Errorf("Error on check stat of .syncignore: %+v", err)
	}
	return nil
}

func (s *Sync) Stop() {
	if s.IsStopped() {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.setState("stopped")
	s.dirty = false

	s.stopRemoteWatcher()
	s.stopUnisonCmd()

	if s.stopCh != nil {
		s.stopCh <- struct{}{}
	}

	s.publishMessage("container.sync-disabled", nil)
}

func (s *Sync) stopDueToError(err error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.setState("stopped")
	s.dirty = false
	s.stopRemoteWatcher()
	s.stopUnisonCmd()
	s.publishMessage("container.sync-disabled", err)
}

func (s *Sync) stopRemoteWatcher() {
	if s.stopRemoteCh != nil {
		done := make(chan struct{})
		go func() {
			s.stopRemoteCh <- struct{}{}
			stopped := <-s.stopRemoteCh
			done <- stopped
		}()

		select {
		case <-done:
			logger.Debugf("sucessfully stopped remote watcher")
		case <-time.After(remoteWatcherTerminateTimeout):
			logger.Errorf("timeout terminating remote watcher")
		}
	}
}

func (s *Sync) stopUnisonCmd() {
	if s.stopUnisonCh != nil {
		done := make(chan struct{})
		go func() {
			s.stopUnisonCh <- struct{}{}
			stopped := <-s.stopUnisonCh
			done <- stopped
		}()

		select {
		case <-done:
			logger.Debugf("sucessfully stopped unison")
		case <-time.After(unisonTerminateTimeout):
			logger.Errorf("timeout terminating unison")
		}
	}
}

func (s *Sync) setState(state string) {
	s.state = state
}

func (s *Sync) IsStopped() bool {
	return s.state == "stopped"
}

func (s *Sync) publishMessage(title string, err error) {
	message := map[string]string{
		"slug": s.containerSlug,
	}

	if err != nil {
		message["error"] = err.Error()
	}

	logger.Event(title, message)
	if err != nil {
		logger.Infof("Published message %s with %+v: %s", title, message, err)
	} else {
		logger.Infof("Published message %s with %+v", title, message)
	}
}
