package sync

import (
	"fmt"
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

	log "github.com/Sirupsen/logrus"
	"github.com/kardianos/osext"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
	"github.com/nitrous-io/nsync/pkg/envmap"
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

func init() {
	createSymlinkIfNotExist := func() {
		sshExeRealPath := filepath.Join(exeFolderPath, "nitrous-ssh")
		sshExeSymlinkPath := filepath.Join(tempFolderPath, "nitrous-ssh")
		if _, err := os.Stat(sshExeSymlinkPath); os.IsNotExist(err) {
			if err := os.Symlink(sshExeRealPath, sshExeSymlinkPath); err != nil {
				log.Panic("Failed to create symlink of nitrous-ssh: %+v", err)
			}
		} else if err != nil {
			log.Panic("Failed to get symlink of nitrous-ssh: %+v", err)
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
			log.Panicf("Failed to genereate unison exec path: %+v", err)
		}
		sshExePath, err = shortname.Get(filepath.Join(exeFolderPath, "nitrous-ssh.exe"))
		if err != nil {
			log.Panicf("Failed to genereate ssh exec path: %+v", err)
		}
	} else {
		log.Panic("Unsupported platform: %s-%s!", runtime.GOOS, runtime.GOARCH)
	}

	for i, p := range ignorePathList {
		ignorePathList[i] = filepath.Join(strings.Split(p, "/")...)
	}
}

func mustString(s string, err error) string {
	if err != nil {
		log.Panic(err)
	}
	return s
}

type Sync struct {
	token             string
	nitrousFolderPath string
	containerSlug     string
	remotePath        string
	sshKeyPath        string
	pubKey            string

	sshUrl   string
	hostSlug string

	dirty       bool
	syncingLock sync.Mutex
	syncing     bool

	shouldIgnoreArchives bool
	shouldIgnoreLocks    bool

	lastError        error
	lastErrorDetails string
	errorIsFatal     bool
	retryDelay       time.Duration

	lastTransferCount int

	syncIgnoreList []string

	stopCh chan struct{}
}

func NewSync(token, sshKeyPath, nitrousFolderPath, containerSlug, remotePath, pubKey string) *Sync {
	s := &Sync{
		token:             token,
		sshKeyPath:        sshKeyPath,
		nitrousFolderPath: nitrousFolderPath,
		containerSlug:     containerSlug,
		remotePath:        remotePath,
		pubKey:            pubKey,
	}

	if err := nitrousapi.UploadPublicKey(s.token, s.containerSlug, s.pubKey); err != nil {
		log.Panicf("sync: Failed upload ssh key: %+v ", err)
	}

	hostId, sshUrl, err := nitrousapi.GetContainerInfo(s.token, s.containerSlug)
	if err != nil {
		log.Panicf("Failed to get container info: %+v", err)
	}
	log.Debug("host id obtained: ", hostId)
	log.Debug("SSH URL obtained: ", sshUrl)
	s.sshUrl = sshUrl

	hostSlug, _, err := nitrousapi.GetHostInfo(token, hostId)
	if err != nil {
		log.Panicf("Failed to get host info: %+v", err)
	}
	log.Debug("host slug obtained: ", hostSlug)
	s.hostSlug = hostSlug

	s.stopCh = make(chan struct{})
	return s
}

func (s *Sync) makeLocalPath() string {
	return filepath.Join(s.nitrousFolderPath, s.hostSlug, s.containerSlug)
}

func (s *Sync) Start() {
	localPath := s.makeLocalPath()

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := os.MkdirAll(localPath, 0700); err != nil {
			log.Panicf("Failed to create folder: %+v", err)
		}

		s.shouldIgnoreArchives = true
	}

	err := s.uploadSyncIgnore()
	if err != nil {
		log.Panicf("Failed to upload .syncignore: %+v", err)
	}

	runSync := func() {
		s.syncingLock.Lock()
		if s.syncing {
			s.dirty = true
		} else {
			s.syncing = true
			go func() {
				s.performSync()
				s.syncingLock.Lock()
				s.syncing = false
				s.syncingLock.Unlock()
			}()
		}
		s.syncingLock.Unlock()
	}

	s.performSync()

	notifyChan := make(chan notify.EventInfo, 1000)
	remoteChan := make(chan string, 1000)
	errorChan := make(chan error, 1)

	var timer *time.Timer
	handleChange := func(changedPath string, local bool) {
		if !pathmatch.PathMatchAny(changedPath, append(ignoreList, ignorePathList...)...) {
			log.Debug("changed path ")
			if local {
				log.Debug("(local): ")
			} else {
				log.Debug("(remote): ")
			}
			log.Debug(changedPath)

			if timer != nil {
				timer.Reset(fileSyncDelay)
			} else {
				timer = time.AfterFunc(fileSyncDelay, runSync)
			}
		}
	}

	log.Debugf("Watching %s/%s...", s.sshUrl, s.remotePath)
	go s.startRemoteWatcher(remoteChan, errorChan, s.stopCh)

	log.Debugf("Watching %s...", localPath)
	if err := notify.Watch(filepath.Join(localPath, "..."), notifyChan, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(notifyChan)

	for {
		select {
		case change := <-notifyChan: // remote file system change
			{
				handleChange(change.Path(), true)
			}
		case change := <-remoteChan:
			{
				match := remoteChangeRe.FindStringSubmatch(change)
				if len(match) >= 2 {
					handleChange(match[1], false)
				}
			}
		case err := <-errorChan:
			{
				log.Fatalf("Lost connection to server: %+v", err)
			}
		case <-s.stopCh:
			{
				log.Infof("Stop syncing...")
				return
			}
		}
	}
}

func (s *Sync) performSync() {
	log.Debug("sync: Begin...")
	err := s.parseSyncIgnore()
	if err != nil {
		log.Panicf("sync: Failed to parse .syncignore: %+v", err)
	}

	s.lastError = nil
	s.lastErrorDetails = ""
	s.errorIsFatal = false
	s.retryDelay = 10 * time.Second
	s.dirty = false
	s.lastTransferCount = -1

	cmd := s.unisonCmd()
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	if err != nil {
		s.lastError = err
	}

	s.handleOutput(outStr)

	if s.lastError != nil {
		log.Errorf("sync: Error occurred: %v", s.lastError)

		if s.errorIsFatal {
			log.Fatal("Error is fatal! Exiting!")
		}

		if s.lastError == ErrSSHAccessDenied {
			if err := nitrousapi.UploadPublicKey(s.token, s.containerSlug, s.pubKey); err != nil {
				log.Panicf("sync: Failed upload ssh key: %+v ", err)
			}
		}

		time.AfterFunc(s.retryDelay, func() {
			log.Info("sync: Retrying...")
			s.performSync()
		})
	}

	if s.lastTransferCount != -1 {
		log.Infof("sync: %d files transferred.", s.lastTransferCount)
	}

	log.Debug("Sync end!")

	if s.dirty {
		log.Debug("changes happened while syncing, syncing again...")
		s.performSync()
	}
}

func (s *Sync) unisonCmd() *exec.Cmd {
	localPath := s.makeLocalPath()
	sshRemotePath := fmt.Sprintf("%s/%s", s.sshUrl, s.remotePath)

	var devNull string
	if isWindows {
		devNull = "NUL"
	} else {
		devNull = "/dev/null"
	}

	var sshKeyPathEscaped string
	if isWindows {
		var err error
		sshKeyPathEscaped, err = shortname.Get(s.sshKeyPath)
		if err != nil {
			log.Panicf("sync: Failed get short name for ssh key path: %+v ", err)
		}
	} else {
		sshKeyPathEscaped = strings.Replace(strings.Replace(s.sshKeyPath, `\`, `\\`, -1), " ", `\ `, -1)
	}

	sshArgs := []string{
		fmt.Sprintf("-o UserKnownHostsFile=%s", devNull),
		"-o StrictHostKeyChecking=no",
		"-o PreferredAuthentications=publickey",
		fmt.Sprintf("-i %s", sshKeyPathEscaped),
	}

	args := []string{
		localPath,                // path to watch
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

	if _, err := os.Stat(localPath); os.IsNotExist(err) || s.shouldIgnoreArchives {
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

	log.Debugf("%s %s", unisonExePath, strings.Join(args, " "))

	cmd := exec.Command(unisonExePath, args...)

	env := envmap.EnvToMap(os.Environ())
	env["UNISON"] = filepath.Join(localPath, ".unison")

	cmd.Env = append(cmd.Env, envmap.MapToEnv(env)...)
	cmd.Dir = exeFolderPath

	return cmd
}

func (s *Sync) uploadSyncIgnore() error {
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

	syncIgnoreLocalPath := filepath.Join(s.makeLocalPath(), syncIgnoreFile)
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

		log.Debug("Downloading .syncignore...")
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
	syncIgnorePath := filepath.Join(s.makeLocalPath(), syncIgnoreFile)
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
	} else {
		log.Errorf("Failed to get .syncignore file information: %+v", err)
	}
	return nil
}

func (s *Sync) Stop() {
	s.stopCh <- struct{}{}
	log.Infof("sync is stopped: %s", s.containerSlug)
}
