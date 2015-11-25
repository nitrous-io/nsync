package nsync

import (
	"errors"
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

	errRemoteWatcherKilled = errors.New("remote watcher killed")
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

type Nsync struct {
	token             string
	nitrousFolderPath string
	containerSlug     string
	remotePath        string
	sshKeyPath        string

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

	stopCh chan struct{}

	syncIgnoreList []string
}

func NewNsync(token, nitrousFolderPath, containerSlug, remotePath string, stopCh chan struct{}) *Nsync {
	ns := &Nsync{
		token:             token,
		nitrousFolderPath: nitrousFolderPath,
		containerSlug:     containerSlug,
		remotePath:        remotePath,
		stopCh:            stopCh,
	}

	username, err := ns.getUsername()
	if err != nil {
		log.Panicf("Failed to get username: %+v", err)
	}

	log.Debug("username obtained: ", username)

	ns.sshKeyPath = filepath.Join(nitrousFolderPath, "SSH Keys", username, "id_rsa")

	if err := ns.generateAndUploadSSHKey(); err != nil {
		log.Panicf("Failed to generate and upload ssh key: %+v", err)
	}

	hostId, sshUrl, err := ns.getContainerInfo(ns.containerSlug)
	if err != nil {
		log.Panicf("Failed to get container info: %+v", err)
	}
	log.Debug("host id obtained: ", hostId)
	log.Debug("SSH URL obtained: ", sshUrl)
	ns.sshUrl = sshUrl

	hostSlug, _, err := ns.getHostInfo(hostId)
	if err != nil {
		log.Panicf("Failed to get host info: %+v", err)
	}
	log.Debug("host slug obtained: ", hostSlug)
	ns.hostSlug = hostSlug

	return ns
}

func (ns *Nsync) generateAndUploadSSHKey() error {
	pubKey, err := ns.generateSSHKeyPairIfAbsent()
	if err != nil {
		return err
	}
	err = ns.uploadPublicKey(ns.containerSlug, pubKey)
	if err != nil {
		return err
	}
	return nil
}

func (ns *Nsync) makeLocalPath() string {
	return filepath.Join(ns.nitrousFolderPath, ns.hostSlug, ns.containerSlug)
}

func (ns *Nsync) Start() {
	localPath := ns.makeLocalPath()

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := os.MkdirAll(localPath, 0700); err != nil {
			log.Panicf("Failed to create folder: %+v", err)
		}

		ns.shouldIgnoreArchives = true
	}

	err := ns.uploadSyncIgnore()
	if err != nil {
		log.Panicf("Failed to upload .syncignore: %+v", err)
	}

	runSync := func() {
		ns.syncingLock.Lock()
		if ns.syncing {
			ns.dirty = true
		} else {
			ns.syncing = true
			go func() {
				ns.performSync()
				ns.syncingLock.Lock()
				ns.syncing = false
				ns.syncingLock.Unlock()
			}()
		}
		ns.syncingLock.Unlock()
	}

	ns.performSync()

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

	log.Debugf("Watching %s/%s...", ns.sshUrl, ns.remotePath)
	go ns.startRemoteWatcher(remoteChan, errorChan, ns.stopCh)

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
		}
	}
}

func (ns *Nsync) performSync() {
	log.Debug("sync: Begin...")
	err := ns.parseSyncIgnore()
	if err != nil {
		log.Panicf("sync: Failed to parse .syncignore: %+v", err)
	}

	ns.lastError = nil
	ns.lastErrorDetails = ""
	ns.errorIsFatal = false
	ns.retryDelay = 10 * time.Second
	ns.dirty = false
	ns.lastTransferCount = -1

	cmd := ns.unisonCmd()
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	if err != nil {
		ns.lastError = err
	}

	ns.handleOutput(outStr)

	if ns.lastError != nil {
		log.Errorf("sync: Error occurred: %v", ns.lastError)

		if ns.errorIsFatal {
			log.Fatal("Error is fatal! Exiting!")
		}

		if ns.lastError == ErrSSHAccessDenied {
			if err := ns.generateAndUploadSSHKey(); err != nil {
				log.Panicf("sync: Failed generate and upload ssh key: %+v ", err)
			}
		}

		time.AfterFunc(ns.retryDelay, func() {
			log.Info("sync: Retrying...")
			ns.performSync()
		})
	}

	if ns.lastTransferCount != -1 {
		log.Infof("sync: %d files transferred.", ns.lastTransferCount)
	}

	log.Debug("Sync end!")

	if ns.dirty {
		log.Debug("changes happened while syncing, syncing again...")
		ns.performSync()
	}
}

func (ns *Nsync) unisonCmd() *exec.Cmd {
	localPath := ns.makeLocalPath()
	sshRemotePath := fmt.Sprintf("%s/%s", ns.sshUrl, ns.remotePath)

	var devNull string
	if isWindows {
		devNull = "NUL"
	} else {
		devNull = "/dev/null"
	}

	var sshKeyPathEscaped string
	if isWindows {
		var err error
		sshKeyPathEscaped, err = shortname.Get(ns.sshKeyPath)
		if err != nil {
			log.Panicf("sync: Failed get short name for ssh key path: %+v ", err)
		}
	} else {
		sshKeyPathEscaped = strings.Replace(strings.Replace(ns.sshKeyPath, `\`, `\\`, -1), " ", `\ `, -1)
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

	if _, err := os.Stat(localPath); os.IsNotExist(err) || ns.shouldIgnoreArchives {
		ns.shouldIgnoreArchives = false
		args = append(args, "-ignorearchives")
	}

	if ns.shouldIgnoreLocks {
		ns.shouldIgnoreLocks = false
		args = append(args, "-ignorelocks")
	}

	if len(ns.syncIgnoreList) > 0 {
		for i := 0; i < len(ns.syncIgnoreList); i++ {
			args = append(args, "-ignore", ns.syncIgnoreList[i])
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

func (ns *Nsync) uploadSyncIgnore() error {
	// Copy .syncignore from remote
	host, config, err := sshconfig.Config(ns.sshUrl, ns.sshKeyPath)
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

	syncIgnoreLocalPath := filepath.Join(ns.makeLocalPath(), syncIgnoreFile)
	syncIgnoreRemotePath := filepath.Join(ns.remotePath, syncIgnoreFile)

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

func (ns *Nsync) parseSyncIgnore() error {
	syncIgnorePath := filepath.Join(ns.makeLocalPath(), syncIgnoreFile)
	if _, err := os.Stat(syncIgnorePath); err == nil {
		ignoreContent, err := ioutil.ReadFile(syncIgnorePath)
		if err != nil {
			return err
		}
		syncIgnoreList := syncignore.Parse(string(ignoreContent))
		if err != nil {
			return err
		}
		ns.syncIgnoreList = append(ns.syncIgnoreList, syncIgnoreList...)
	} else {
		log.Errorf("Failed to get .syncignore file information: %+v", err)
	}
	return nil
}

func (ns *Nsync) StartServer() {
	go NewServer(ns.sshKeyPath).Start()
}
