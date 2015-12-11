// +build windows
// +build 386

package pidfile

import (
	"os"
)

type LockFile struct {
	*os.File
	name string
}

func NewLockFile(name string, perm os.FileMode) (*LockFile, error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_EXCL, perm)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lockfile := &LockFile{f, name}
	return lockfile, nil
}

// Remove removes lock, closes and removes an open file.
func (file *LockFile) Remove() error {
	return os.Remove(file.name)
}
