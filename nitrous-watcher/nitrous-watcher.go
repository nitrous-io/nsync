// +build linux darwin

package main

import (
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"golang.org/x/exp/inotify"
)

var (
	ignoreRegex *regexp.Regexp
)

func recursivelyWatchDirectories(watcher *inotify.Watcher, path string) {
	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() {
			if ignoreRegex != nil && ignoreRegex.MatchString(path) {
				return filepath.SkipDir
			}
			err = watcher.AddWatch(path, inotify.IN_MODIFY|inotify.IN_CREATE|inotify.IN_DELETE|inotify.IN_MOVE|inotify.IN_DELETE_SELF|inotify.IN_CLOSE_WRITE)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				if strings.Contains(err.Error(), "no space left on device") {
					os.Exit(1)
				}
			}
		}
		return nil
	})
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func main() {
	watcher, err := inotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s [path] ([ignore regex])\n", os.Args[0])
		os.Exit(1)
	}

	if len(os.Args) >= 3 {
		ignoreRegex = regexp.MustCompile(os.Args[2])
	}

	watchDir := os.Args[1]

	if strings.HasPrefix(watchDir, "~/") {
		cUsr, err := user.Current()
		if err == nil {
			watchDir = cUsr.HomeDir + "/" + strings.TrimPrefix(watchDir, "~/")
		}
	}

	if finalDir, err := filepath.EvalSymlinks(watchDir); err == nil {
		watchDir = finalDir
	}
	recursivelyWatchDirectories(watcher, watchDir)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		for _ = range c {
			watcher.Close()
			os.Exit(0)
		}
	}()

	fmt.Printf("Ready to watch %s\n", watchDir)
	for {
		select {
		case ev := <-watcher.Event:
			evName := ""
			evMask := ev.Mask
			path := ev.Name
			if evMask&inotify.IN_MODIFY != 0 {
				evName = "modify"
			} else if evMask&inotify.IN_CREATE != 0 {
				evName = "create"
				if evMask&inotify.IN_ISDIR != 0 {
					recursivelyWatchDirectories(watcher, path)
				}
			} else if evMask&inotify.IN_DELETE != 0 || evMask&inotify.IN_DELETE_SELF != 0 {
				evName = "delete"
				if evMask&inotify.IN_ISDIR != 0 {
					watcher.RemoveWatch(path)
				}
			} else if evMask&inotify.IN_MOVE != 0 {
				evName = "move"
				found, err := exists(path)
				// move generates 2 events, one for the old directory being moved and one for the new location
				if found {
					if evMask&inotify.IN_ISDIR != 0 {
						recursivelyWatchDirectories(watcher, path)
					}
				} else if err != nil {
					watcher.RemoveWatch(path)
				}
			} else if evMask&inotify.IN_CLOSE_WRITE != 0 {
				evName = "closed"
			}
			fmt.Printf("Change(%s): %s\n", evName, path)
		}
	}
}
