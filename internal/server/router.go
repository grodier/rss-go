package server

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/grodier/rss-go/internal/ui"
)

func (s *Server) router() http.Handler {
	router := chi.NewRouter()

	// Global middleware applied to all routes
	router.Use(s.recoverPanic)
	router.Use(s.logRequest)
	router.Use(commonHeaders)

	// Static files - serve from the "static" subdirectory of the embedded FS
	staticFS, err := fs.Sub(ui.NoDirFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServerFS(staticFS)
	router.Handle("/static/*", http.StripPrefix("/static", fileServer))

	// Health check
	router.Get("/api/v1/healthcheck", s.handleHealthcheck)

	// Unprotected routes with session, CSRF, and auth check
	router.Group(func(r chi.Router) {
		r.Use(s.sessionManager.LoadAndSave)
		r.Use(s.preventCSRF)
		r.Use(s.authenticate)

		r.Get("/", s.handleRootView)
		r.Get("/about", s.handleAboutView)
		r.Get("/login", s.handleUserLogin)
		r.Post("/login", s.handleUserLoginPost)
		r.Get("/signup", s.handleUserSignUp)
		r.Post("/signup", s.handleUserSignUpPost)
	})

	// Protected routes requiring authentication
	router.Group(func(r chi.Router) {
		r.Use(s.sessionManager.LoadAndSave)
		r.Use(s.preventCSRF)
		r.Use(s.authenticate)
		r.Use(s.requireAuthentication)

		r.Get("/feeds", s.handleFeedsList)
		r.Get("/feeds/search", s.handleFeedSearch)
		r.Post("/feeds/search", s.handleFeedSearchPost)
		r.Get("/feeds/suggest", s.handleFeedSuggest)
		r.Get("/feeds/discover/{id}/events", s.handleFeedDiscoveryEvents)
		r.Get("/feeds/{id}", s.handleFeedView)
		r.Get("/feeds/new", s.handleFeedCreate)
		r.Post("/feeds/new", s.handleFeedCreatePost)
		r.Post("/user/logout", s.handleUserLogoutPost)
	})

	return router
}
