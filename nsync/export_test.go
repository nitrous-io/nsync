package nsync

import "github.com/gorilla/mux"

func (s *Server) Router() *mux.Router {
	return s.router
}
