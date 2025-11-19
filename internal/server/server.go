package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grodier/rss-go/internal/models"
	"github.com/grodier/rss-go/internal/tmpl"
	"github.com/grodier/rss-go/internal/validator"
)

type Server struct {
	Port int
	Env  string

	FeedService models.FeedService

	template *template.Template
	server   *http.Server
	logger   *slog.Logger
}

func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		template: tmpl.NewTmpl(),
		server:   &http.Server{},
		logger:   logger,
	}

	return s
}

func (s *Server) Serve() error {
	s.server.Handler = s.router()
	s.server.Addr = fmt.Sprintf(":%d", s.Port)
	s.server.IdleTimeout = time.Minute
	s.server.ReadTimeout = 5 * time.Second
	s.server.WriteTimeout = 10 * time.Second

	shutdownError := make(chan error)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit

		s.logger.Info("caught signal", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := s.server.Shutdown(ctx)
		shutdownError <- err
	}()

	s.logger.Info("starting server", "addr", s.Port, "env", s.Env)

	err := s.server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	err = <-shutdownError
	if err != nil {
		return err
	}

	s.logger.Info("stopped server", "addr", s.server.Addr, "env", s.Env)

	return nil
}

func (s *Server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleRootView(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.FeedService.GetLatestFeeds()
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	data := struct {
		Feeds []*models.Feed
	}{
		Feeds: feeds,
	}

	s.render(w, r, http.StatusOK, "root.tmpl.html", data)
}

func (s *Server) handleFeedView(w http.ResponseWriter, r *http.Request) {
	id, err := s.readIDParam(r)

	// in future want to return a better msg/page to user
	// and wrap the functionality in utility of some sort
	if err != nil {
		http.NotFound(w, r)
		return
	}

	feed, err := s.FeedService.GetFeedByID(int(id))
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			s.serverError(w, r, err)
		}
		return
	}

	data := struct {
		ID          int
		Title       string
		Description string
	}{
		ID:          feed.ID,
		Title:       feed.Title,
		Description: feed.Description,
	}

	s.render(w, r, http.StatusOK, "feed.tmpl.html", data)
}

type feedCreateData struct {
	Feed        models.Feed
	FieldErrors map[string]string
}

func (s *Server) handleFeedCreate(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "feed_create.tmpl.html", feedCreateData{})
}

// TODO: consider moving validation to feed creation and reduce handler responsibilities
func (s *Server) handleFeedCreatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	var newFeed models.Feed

	newFeed.Title = r.PostForm.Get("title")
	newFeed.Description = r.PostForm.Get("description")

	var v validator.Validator

	v.Check(validator.NotBlank(newFeed.Title), "title", "This field cannot be blank")
	v.Check(validator.MaxChars(newFeed.Title, 100), "title", "This field cannot be more than 100 characters long")
	v.Check(validator.NotBlank(newFeed.Description), "description", "This field cannot be blank")

	if !v.Valid() {
		data := feedCreateData{
			Feed:        newFeed,
			FieldErrors: v.FieldErrors,
		}

		s.render(w, r, http.StatusUnprocessableEntity, "feed_create.tmpl.html", data)
		return
	}

	if err := s.FeedService.CreateFeed(&newFeed); err != nil {
		s.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/feed/view/%d", newFeed.ID), http.StatusSeeOther)
}
