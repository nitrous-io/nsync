package nsync

import (
	"github.com/gorilla/mux"
	"github.com/nitrous-io/nsync/nsync/nitrousapi"
)

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) SetSSHInfo(token, nitrousPath, sshKeyPath, pubKey string) {
	s.token = token
	s.nitrousPath = nitrousPath
	s.sshKeyPath = sshKeyPath
	s.pubKey = pubKey

	s.nitrousAPI = nitrousapi.NewNitrousAPI(token, "https://test.example")
}
