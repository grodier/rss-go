package server

import (
	"net/http"

	"github.com/grodier/rss-go/internal/ui"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
)

func (s *Server) router() http.Handler {
	router := httprouter.New()

	router.Handler(http.MethodGet, "/static/*file", http.FileServerFS(ui.NoDirFiles))
	router.HandlerFunc(http.MethodGet, "/api/v1/healthcheck", s.handleHealthcheck)

	dynamic := alice.New(s.sessionManager.LoadAndSave)

	router.Handler(http.MethodGet, "/", dynamic.ThenFunc(s.handleRootView))
	router.Handler(http.MethodGet, "/feed/view/:id", dynamic.ThenFunc(s.handleFeedView))
	router.Handler(http.MethodGet, "/feed/create", dynamic.ThenFunc(s.handleFeedCreate))
	router.Handler(http.MethodPost, "/feed/create", dynamic.ThenFunc(s.handleFeedCreatePost))

	standardHeaders := alice.New(s.recoverPanic, s.logRequest, commonHeaders)

	return standardHeaders.Then(router)
}
