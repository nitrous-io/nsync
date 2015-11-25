package nsync

import (
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
)

type Nsync struct {
	token             string
	sshKeyPath        string
	nitrousFolderPath string
	pubKey            string

	stopCh chan struct{}
}

func NewNsync(token, nitrousFolderPath string, stopCh chan struct{}) *Nsync {
	ns := &Nsync{
		token:             token,
		nitrousFolderPath: nitrousFolderPath,
		stopCh:            stopCh,
	}

	username, err := nitrousapi.GetUsername(token)
	if err != nil {
		log.Panicf("Failed to get username: %+v", err)
	}

	log.Debug("username obtained: ", username)

	ns.sshKeyPath = filepath.Join(nitrousFolderPath, "SSH Keys", username, "id_rsa")
	ns.pubKey, err = ns.generateSSHKeyPairIfAbsent()
	if err != nil {
		log.Panicf("Failed to generate ssh key: %+v", err)
	}

	return ns
}

func (ns *Nsync) StartServer() {
	go NewServer(ns.token, ns.sshKeyPath, ns.nitrousFolderPath, ns.pubKey).Start()
}
