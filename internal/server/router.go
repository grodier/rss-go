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

	dynamic := alice.New(s.sessionManager.LoadAndSave, s.preventCSRF)

	// Unprotected routes
	router.Handler(http.MethodGet, "/", dynamic.ThenFunc(s.handleRootView))
	router.Handler(http.MethodGet, "/feed/view/:id", dynamic.ThenFunc(s.handleFeedView))
	router.Handler(http.MethodGet, "/user/login", dynamic.ThenFunc(s.handleUserLogin))
	router.Handler(http.MethodPost, "/user/login", dynamic.ThenFunc(s.handleUserLoginPost))
	router.Handler(http.MethodGet, "/user/signup", dynamic.ThenFunc(s.handleUserSignUp))
	router.Handler(http.MethodPost, "/user/signup", dynamic.ThenFunc(s.handleUserSignUpPost))

	protected := dynamic.Append(s.requireAuthentication)

	// Protected routes
	router.Handler(http.MethodGet, "/feed/create", protected.ThenFunc(s.handleFeedCreate))
	router.Handler(http.MethodPost, "/feed/create", protected.ThenFunc(s.handleFeedCreatePost))
	router.Handler(http.MethodPost, "/user/logout", protected.ThenFunc(s.handleUserLogoutPost))

	standardHeaders := alice.New(s.recoverPanic, s.logRequest, commonHeaders)

	return standardHeaders.Then(router)
}
