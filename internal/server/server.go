package server

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

type Server struct {
	server *http.Server
}

func NewServer() *Server {
	s := &Server{
		server: &http.Server{},
	}

	return s
}

func (s *Server) Serve() error {
	s.server.Handler = s.routes()
	s.server.Addr = ":8080"
	return s.server.ListenAndServe()
}

func (s *Server) routes() http.Handler {
	router := httprouter.New()

	router.HandlerFunc(http.MethodGet, "/api/v1/healthcheck", s.handleHealthcheck)

	return router
}

func (s *Server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
