package nsync

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/nitrous-io/nsync/pkg/sshkeygen"
)

var ErrInvalidPublicKey = errors.New("invalid public key")
var ErrUnmatchedPublicKey = errors.New("public key does not match private key")

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

	// Public key and Private are present
	publicKey, err := ioutil.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}

	pubKey = string(publicKey)
	if ok, err := ns.verifyPublicKey(name, pubKey); !ok || err != nil {
		return "", ErrInvalidPublicKey
	}

	return pubKey, nil
}

func (ns *Nsync) verifyPublicKey(name, pubKey string) (bool, error) {
	privKey, err := ioutil.ReadFile(ns.sshKeyPath)
	if err != nil {
		return false, err
	}

	generatedPublicKeyFromPrivateKey, err := sshkeygen.GeneratePubKeyUsingPrivKeyPEM(privKey, name)
	if err != nil {
		return false, err
	}

	fingerprintedExistingPublicKey, err := md5Fingerprint(pubKey)
	if err != nil {
		return false, err
	}

	fingerprintedGeneratedPublicKey, err := md5Fingerprint(generatedPublicKeyFromPrivateKey)
	if err != nil {
		return false, err
	}

	if fingerprintedExistingPublicKey != fingerprintedGeneratedPublicKey {
		return false, ErrUnmatchedPublicKey
	}

	return true, nil
}

func md5Fingerprint(key string) (string, error) {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(key)))

	if err != nil {
		return "", ErrInvalidPublicKey
	}

	md5sum := md5.Sum(pk.Marshal())
	buf := &bytes.Buffer{}
	for _, v := range md5sum {
		buf.WriteString(fmt.Sprintf("%02x:", v))
	}

	return buf.String()[:47], nil
}
