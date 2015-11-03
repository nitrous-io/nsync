package nsync

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/osext"
	"github.com/nitrous-io/nsync/pkg/envmap"
	"github.com/nitrous-io/nsync/pkg/pathmatch"
	"github.com/nitrous-io/nsync/pkg/shortname"
	"github.com/rjeczalik/notify"
)

var exeFolderPath = mustString(osext.ExecutableFolder())
var unisonExePath, sshExePath string
var isWindows = runtime.GOOS == "windows"

var ignoreList = []string{".unison", ".git", ".svn", ".hg", "CVS", ".DS_Store", ".AppleDouble", ".LSOverride", ".Spotlight-V100", ".Trashes", "Thumbs.db", "desktop.ini", "*.lock", "*.sock", "*.pid", "*.sw?", "._*", "*~"}
var ignorePathList = []string{"tmp/sessions", "tmp/cache", "tmp/pids", "tmp/sockets"}

const fileSyncDelay = 100 * time.Millisecond

func init() {
	if runtime.GOOS == "darwin" {
		unisonExePath = filepath.Join(exeFolderPath, "bin", "osx64", "nitrous-unison")
		sshExePath = filepath.Join(exeFolderPath, "bin", "osx64", "nitrous-ssh")
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		unisonExePath = filepath.Join(exeFolderPath, "bin", "lin64", "nitrous-unison")
		sshExePath = filepath.Join(exeFolderPath, "bin", "lin64", "nitrous-unison")
	} else if runtime.GOOS == "windows" {
		unisonExePath = filepath.Join(exeFolderPath, "bin", "win32", "nitrous-unison.exe")
		var err error
		sshExePath, err = shortname.Get(filepath.Join(exeFolderPath, "bin", "win32", "nitrous-ssh.exe"))
		if err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Sprintf("Unsupported platform: %s-%s!", runtime.GOOS, runtime.GOARCH))
	}

	for i, p := range ignorePathList {
		ignorePathList[i] = filepath.Join(strings.Split(p, "/")...)
	}
}

func mustString(s string, err error) string {
	if err != nil {
		panic(err)
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
}

func NewNsync(token, nitrousFolderPath, containerSlug, remotePath string) *Nsync {
	ns := &Nsync{
		token:             token,
		nitrousFolderPath: nitrousFolderPath,
		containerSlug:     containerSlug,
		remotePath:        remotePath,
	}

	username, err := ns.getUsername()
	if err != nil {
		panic(err)
	}
	fmt.Println("username obtained: ", username)

	ns.sshKeyPath = filepath.Join(nitrousFolderPath, "SSH Keys", username, "id_rsa")

	hostId, sshUrl, err := ns.getContainerInfo(ns.containerSlug)
	if err != nil {
		panic(err)
	}
	fmt.Println("host id obtained: ", hostId)
	fmt.Println("SSH URL obtained: ", sshUrl)
	ns.sshUrl = sshUrl

	hostSlug, _, err := ns.getHostInfo(hostId)
	if err != nil {
		panic(err)
	}
	fmt.Println("host slug obtained: ", hostSlug)
	ns.hostSlug = hostSlug

	return ns
}

func (ns *Nsync) makeLocalPath() string {
	return filepath.Join(ns.nitrousFolderPath, ns.hostSlug, ns.containerSlug)
}

func (ns *Nsync) Start() {
	localPath := ns.makeLocalPath()

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := os.MkdirAll(localPath, 0700); err != nil {
			panic(err)
		}

		ns.shouldIgnoreArchives = true
	}

	var timer *time.Timer

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

	fsC := make(chan notify.EventInfo, 1000)

	fmt.Printf("Watching %s...\n", localPath)
	if err := notify.Watch(filepath.Join(localPath, "..."), fsC, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsC)

	for {
		select {
		case change := <-fsC: // file system change
			changedPath := change.Path()

			if !pathmatch.PathMatchAny(changedPath, append(ignoreList, ignorePathList...)...) {
				fmt.Println("Detected change:", change)

				if timer != nil {
					timer.Reset(fileSyncDelay)
				} else {
					timer = time.AfterFunc(fileSyncDelay, runSync)
				}
			}
		}
	}
}

func (ns *Nsync) performSync() {
	log.Println("Sync begin...")
	ns.lastError = nil
	ns.lastErrorDetails = ""
	ns.errorIsFatal = false
	ns.retryDelay = 10 * time.Second
	ns.dirty = false

	cmd := ns.unisonCmd()
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	fmt.Println(outStr)

	if err != nil {
		ns.lastError = err
	}

	ns.handleOutput(outStr)

	if ns.lastError != nil {
		log.Printf("Error occurred: %v", ns.lastError)

		if ns.errorIsFatal {
			log.Fatal("Error is fatal! Exiting!")
		}

		time.AfterFunc(ns.retryDelay, func() {
			log.Println("Retrying...")
			ns.performSync()
		})
	}

	log.Println("Sync end!")

	if ns.dirty {
		log.Println("changes happened while syncing, syncing again...")
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
			panic(err)
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
		"-log=false",           // no log messages
		"-rsrc=false",          // don't sync resource forks
		"-sshcmd", sshExePath,  // local ssh command path
		"-sshargs", strings.Join(sshArgs, " "),
		"-servercmd=/nitrous/nitrous-unison",
		"-ignore", fmt.Sprintf("Name {%s}", strings.Join(ignoreList, ",")),
		"-ignore", fmt.Sprintf("Path {%s}", strings.Join(ignorePathList, ",")),
		"-ignorecase=true",
	}

	fmt.Printf("should ignore archives: %v\n", ns.shouldIgnoreArchives)

	if _, err := os.Stat(localPath); os.IsNotExist(err) || ns.shouldIgnoreArchives {
		ns.shouldIgnoreArchives = false
		args = append(args, "-ignorearchives")
	}

	if ns.shouldIgnoreLocks {
		ns.shouldIgnoreLocks = false
		args = append(args, "-ignorelocks")
	}

	fmt.Printf("%s %s\n", unisonExePath, strings.Join(args, " "))

	cmd := exec.Command(unisonExePath, args...)

	env := envmap.EnvToMap(os.Environ())
	env["UNISON"] = filepath.Join(localPath, ".unison")

	cmd.Env = append(cmd.Env, envmap.MapToEnv(env)...)
	cmd.Dir = exeFolderPath

	return cmd
}
