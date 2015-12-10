package hidefiles

import (
	"os/exec"
	"runtime"
)

func Hide(filepath string) error {
	var command string
	var args []string
	if runtime.GOOS == "darwin" {
		command = "chflags"
		args = []string{"hidden", filepath}
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return nil // No way to hide
	} else if runtime.GOOS == "windows" {
		command = "attrib"
		args = []string{"+h", filepath}
	} else {
		return nil // No way to hide
	}

	cmd := exec.Command(command, args...)
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	return nil
}

func Unhide(filepath string) error {
	var command string
	var args []string
	if runtime.GOOS == "darwin" {
		command = "chflags"
		args = []string{"nohidden", filepath}
	} else if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return nil // No way to hide
	} else if runtime.GOOS == "windows" {
		command = "attrib"
		args = []string{"-h", filepath}
	} else {
		return nil // No way to hide
	}

	cmd := exec.Command(command, args...)
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	return nil
}
