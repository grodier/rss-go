package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"

	"github.com/grodier/rss-go/internal/ui"
	"github.com/julienschmidt/httprouter"
)

func (s *Server) router() http.Handler {
	router := httprouter.New()

	router.Handler(http.MethodGet, "/static/*file", http.FileServerFS(noDirListFS{ui.Files, s}))
	router.HandlerFunc(http.MethodGet, "/", s.handleRootView)
	router.HandlerFunc(http.MethodGet, "/feed/view/:id", s.handleFeedView)
	router.HandlerFunc(http.MethodGet, "/feed/create", s.handleFeedCreate)
	router.HandlerFunc(http.MethodPost, "/feed/create", s.handleFeedCreatePost)
	router.HandlerFunc(http.MethodGet, "/api/v1/healthcheck", s.handleHealthcheck)

	return router
}

type noDirListFS struct {
	fs embed.FS
	s  *Server
}

func (n noDirListFS) Open(name string) (fs.File, error) {
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		n.s.logger.Info("attempt to list directory", "directory", name)
		index := filepath.Join(name, "index.html")
		if _, err := n.fs.Open(index); err != nil {
			closeErr := f.Close()
			if closeErr != nil {
				return nil, closeErr
			}

			return nil, err
		}
	}
	return f, nil
}
