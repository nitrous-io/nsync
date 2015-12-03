package nsync

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func (s *Server) Router() *mux.Router {
	return s.router
}

func (s *Server) SetSSHInfo(nitrousPath, sshKeyPath, pubKey string) {
	s.nitrousPath = nitrousPath
	s.sshKeyPath = sshKeyPath
	s.pubKey = pubKey
}

func (s *Server) SetToken(w http.ResponseWriter, token string) {
	w.Header().Set("Authorization", fmt.Sprintf("Bearer %s", token))

}
