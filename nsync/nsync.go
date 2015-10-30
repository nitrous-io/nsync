package nsync

import (
	"fmt"
	"log"
	"path"
	"sync"
	"time"

	"github.com/rjeczalik/notify"
)

const fileSyncDelay = 100 * time.Millisecond

type Nsync struct {
	Token         string
	PathToWatch   string
	ContainerSlug string
	RemotePath    string

	dirty       bool
	syncingLock sync.Mutex
	syncing     bool
}

func NewNsync(token, pathToWatch, containerSlug, remotePath string) *Nsync {
	return &Nsync{
		Token:         token,
		PathToWatch:   pathToWatch,
		ContainerSlug: containerSlug,
		RemotePath:    remotePath,
	}
}

func (ns *Nsync) Start() {
	var timer *time.Timer

	fsC := make(chan notify.EventInfo, 1000)

	if err := notify.Watch(path.Join(ns.PathToWatch, "..."), fsC, notify.All); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(fsC)

	for {
		select {
		case <-fsC: // file system change
			//log.Println("Got event:", ei)
			if timer != nil {
				timer.Reset(fileSyncDelay)
			} else {
				timer = time.AfterFunc(fileSyncDelay, func() {
					timer = nil
					ns.syncingLock.Lock()
					if ns.syncing {
						ns.dirty = true
					} else {
						ns.syncing = true
						go ns.performSync()
					}
					ns.syncingLock.Unlock()
				})
			}
		}
	}
}

func (ns *Nsync) performSync() {
	fmt.Println("starting sync...")
	time.Sleep(4 * time.Second) // simulate sync...
	fmt.Println("end sync...")
	if ns.dirty {
		fmt.Println("changes happened while syncing, syncing again...")
		ns.dirty = false
		ns.performSync()
	}
	ns.syncingLock.Lock()
	ns.syncing = false
	ns.syncingLock.Unlock()
}
