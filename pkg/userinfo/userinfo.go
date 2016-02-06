// https://github.com/golang/go/issues/6376
// user.Current does not work in linux
package userinfo

import (
	"errors"
	"os"
	"os/user"
	"runtime"
)

var (
	errEmptyUsername = errors.New("$USER is empty")
	errEmptyHomeDir  = errors.New("$HOME is empty")
)

func CurrentUsername() (string, error) {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		username := os.Getenv("USER")
		if username == "" {
			return "", errEmptyUsername
		}

		return username, nil
	}

	u, err := user.Current()
	if err != nil {
		return "", err
	}

	return u.Username, nil
}

func CurrentHomeFolder() (string, error) {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		homeDir := os.Getenv("HOME")
		if homeDir == "" {
			return "", errEmptyHomeDir
		}

		return homeDir, nil
	}

	u, err := user.Current()
	if err != nil {
		return "", err
	}

	return u.HomeDir, nil
}
