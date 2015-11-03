// +build windows
// +build 386

package shortname

import "syscall"

func Get(path string) (string, error) {
	p, err := syscall.UTF16FromString(path)
	if err != nil {
		return "", err
	}
	b := p // GetShortPathName says we can reuse buffer
	n := uint32(len(b))
	for {
		n, err = syscall.GetShortPathName(&p[0], &b[0], uint32(len(b)))
		if err != nil {
			return "", err
		}
		if n <= uint32(len(b)) {
			return syscall.UTF16ToString(b[:n]), nil
		}
		b = make([]uint16, n)
	}
}
