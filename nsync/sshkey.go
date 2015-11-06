package nsync

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/nitrous-io/nsync/pkg/sshkeygen"
)

func (ns *Nsync) generateSSHKeyPairIfAbsent() (pubKey string, err error) {
	pubKeyPath := ns.sshKeyPath + ".pub"

	u, err := user.Current()
	if err != nil {
		return "", err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	name := fmt.Sprintf("%s@%s.desktop.nitrous.io", u.Username, hostname)

	if err := os.MkdirAll(filepath.Dir(ns.sshKeyPath), 0700); err != nil {
		return "", err
	}

	if _, err := os.Stat(ns.sshKeyPath); os.IsNotExist(err) {

		privKey, pubKey, err := sshkeygen.Generate(2048, name)
		if err != nil {
			return "", err
		}

		if err := ioutil.WriteFile(ns.sshKeyPath, []byte(privKey), 0600); err != nil {
			return "", err
		}

		if err := ioutil.WriteFile(pubKeyPath, []byte(pubKey), 0644); err != nil {
			return "", err
		}

		return pubKey, nil
	}

	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		privKey, err := ioutil.ReadFile(ns.sshKeyPath)
		if err != nil {
			return "", err
		}

		pubKey, err := sshkeygen.GeneratePubKeyUsingPrivKeyPEM(privKey, name)
		if err != nil {
			return "", err
		}

		if err := ioutil.WriteFile(pubKeyPath, []byte(pubKey), 0644); err != nil {
			return "", err
		}

		return pubKey, nil
	}

	publicKey, err := ioutil.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}

	return string(publicKey), nil
}
