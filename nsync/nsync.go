package nsync

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/osext"
	"github.com/nitrous-io/nsync/pkg/envmap"
	"github.com/nitrous-io/nsync/pkg/pathmatch"
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
		unisonExePath = path.Join(exeFolderPath, "bin", "osx64", "nitrous-unison")
		sshExePath = path.Join(exeFolderPath, "bin", "osx64", "nitrous-ssh")
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		unisonExePath = path.Join(exeFolderPath, "bin", "lin64", "nitrous-unison")
		sshExePath = path.Join(exeFolderPath, "bin", "lin64", "nitrous-unison")
	} else if runtime.GOOS == "windows" {
		unisonExePath = path.Join(exeFolderPath, "bin", "win32", "nitrous-unison.exe")
		sshExePath = path.Join(exeFolderPath, "bin", "win32", "nitrous-ssh.exe")

		for i, p := range ignorePathList {
			ignorePathList[i] = strings.Replace(p, "/", string(os.PathSeparator), -1)
		}
	} else {
		panic(fmt.Sprintf("Unsupported platform: %s-%s!", runtime.GOOS, runtime.GOARCH))
	}
}

func mustString(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

type Nsync struct {
	Token         string
	LocalPath     string
	ContainerSlug string
	RemotePath    string

	dirty       bool
	syncingLock sync.Mutex
	syncing     bool

	shouldIgnoreArchives bool
	shouldIgnoreLocks    bool

	failCount int
	lastError error
}

func NewNsync(token, pathToWatch, containerSlug, remotePath string) *Nsync {
	return &Nsync{
		Token:         token,
		LocalPath:     pathToWatch,
		ContainerSlug: containerSlug,
		RemotePath:    remotePath,
	}
}

func (ns *Nsync) Start() {
	if _, err := os.Stat(ns.LocalPath); os.IsNotExist(err) {
		if err := os.MkdirAll(ns.LocalPath, 0700); err != nil {
			panic(err)
		}

		ns.shouldIgnoreArchives = true
	}

	var timer *time.Timer

	runSync := func() {
		timer = nil
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

	fmt.Printf("Watching %s...\n", ns.LocalPath)
	if err := notify.Watch(path.Join(ns.LocalPath, "..."), fsC, notify.All); err != nil {
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
	fmt.Println("Sync begin...")

	cmd := ns.unisonCmd()
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	fmt.Println(outStr)

	if err != nil {
		ns.failCount++
		ns.lastError = err
		fmt.Println("Sync failed!")
	}

	fmt.Println("Sync end!")
	if ns.dirty {
		fmt.Println("changes happened while syncing, syncing again...")
		ns.dirty = false
		ns.performSync()
	}
}

type unisonCmdOptions struct {
}

func (ns *Nsync) unisonCmd() *exec.Cmd {
	sshRemotePath := fmt.Sprintf("ssh://nitrous@pete-4644.nitrouspro.com:32839/%s", ns.RemotePath)
	sshKeyPath := "/Users/jihoon/workspace/go/src/github.com/nitrous-io/nsync/tmp/id_rsa"

	var devNull string
	if isWindows {
		devNull = "NUL"
	} else {
		devNull = "/dev/null"
	}

	sshArgs := []string{
		fmt.Sprintf("-o UserKnownHostsFile=%s", devNull),
		"-o StrictHostKeyChecking=no",
		"-o PreferredAuthentications=publickey",
		fmt.Sprintf("-i %s", sshKeyPath),
	}

	args := []string{
		ns.LocalPath,             // path to watch
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

	if _, err := os.Stat(ns.LocalPath); os.IsNotExist(err) || ns.shouldIgnoreArchives {
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
	env["UNISON"] = path.Join(ns.LocalPath, ".unison")

	cmd.Env = append(cmd.Env, envmap.MapToEnv(env)...)
	cmd.Dir = exeFolderPath

	return cmd
}
