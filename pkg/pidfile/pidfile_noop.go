// +build linux darwin
// +build 386 amd64

package pidfile

import (
	"os"
	"syscall"
)

// LockFile wraps *os.File and provide functions for locking of files.
type LockFile struct {
	*os.File
	name string
}

// OpenLockFile opens the named file with flags os.O_RDWR|os.O_CREATE and specified perm.
// If successful, function returns LockFile for opened file.
func NewLockFile(name string, perm os.FileMode) (*LockFile, error) {
	var (
		file *os.File
		err  error
		lock *LockFile
	)

	if file, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE, perm); err == nil {
		lock = &LockFile{file, name}
	}

	if err := lock.lock(); err != nil {
		return nil, err
	}

	return lock, nil
}

// Lock apply exclusive lock on an open file. If file already locked, returns error.
func (file *LockFile) lock() error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// Unlock remove exclusive lock on an open file.
func (file *LockFile) unlock() error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

// Remove removes lock, closes and removes an open file.
func (file *LockFile) Remove() error {
	defer file.Close()

	if err := file.unlock(); err != nil {
		return err
	}

	return syscall.Unlink(file.name)
}
