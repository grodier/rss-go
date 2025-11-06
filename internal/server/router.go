package server

import (
	"net/http"

	"github.com/grodier/rss-go/internal/ui"
	"github.com/julienschmidt/httprouter"
)

func (s *Server) router() http.Handler {
	router := httprouter.New()

	router.Handler(http.MethodGet, "/static/*file", http.FileServerFS(ui.NoDirFiles))
	router.HandlerFunc(http.MethodGet, "/", s.handleRootView)
	router.HandlerFunc(http.MethodGet, "/feed/view/:id", s.handleFeedView)
	router.HandlerFunc(http.MethodGet, "/feed/create", s.handleFeedCreate)
	router.HandlerFunc(http.MethodPost, "/feed/create", s.handleFeedCreatePost)
	router.HandlerFunc(http.MethodGet, "/api/v1/healthcheck", s.handleHealthcheck)

	return router
}
