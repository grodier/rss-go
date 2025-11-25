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

	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	"github.com/grodier/rss-go/internal/models"
	"github.com/grodier/rss-go/internal/tmpl"
	"github.com/grodier/rss-go/internal/validator"
)

type Server struct {
	Port int
	Env  string

	FeedService models.FeedService
	UserService models.UserService

	template       *template.Template
	server         *http.Server
	decoder        *form.Decoder
	logger         *slog.Logger
	sessionManager *scs.SessionManager
}

func NewServer(logger *slog.Logger) *Server {
	sessionManager := scs.New()
	sessionManager.Lifetime = 12 * time.Hour

	s := &Server{
		template: tmpl.NewTmpl(),
		server: &http.Server{
			ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		decoder:        form.NewDecoder(),
		logger:         logger,
		sessionManager: sessionManager,
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

type feedViewData struct {
	Feed  models.Feed
	Flash string
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

	data := feedViewData{
		Feed:  *feed,
		Flash: s.sessionManager.PopString(r.Context(), "flash"),
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
	var newFeed models.Feed

	err := s.decodePostForm(r, &newFeed)
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

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

	s.sessionManager.Put(r.Context(), "flash", "Feed successfully created")

	http.Redirect(w, r, fmt.Sprintf("/feed/view/%d", newFeed.ID), http.StatusSeeOther)
}

type userSignUpData struct {
	User        models.UserInput
	FieldErrors map[string]string
}

func (s *Server) handleUserSignUp(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "signup.tmpl.html", userSignUpData{})
}

// TODO: consider moving validation to user sign up and reduce handler responsibilities
func (s *Server) handleUserSignUpPost(w http.ResponseWriter, r *http.Request) {
	var newUser models.UserInput

	err := s.decodePostForm(r, &newUser)
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	var v validator.Validator

	v.Check(validator.NotBlank(newUser.Name), "name", "This field cannot be blank")
	v.Check(validator.NotBlank(newUser.Email), "email", "This field cannot be blank")
	v.Check(validator.Matches(newUser.Email, validator.EmailRX), "email", "This field must be a valid email address")
	v.Check(validator.NotBlank(r.PostForm.Get("password")), "password", "This field cannot be blank")
	v.Check(validator.MinChars(r.PostForm.Get("password"), 8), "password", "This field must be at least 8 characters long")

	if !v.Valid() {
		data := userSignUpData{
			User:        newUser,
			FieldErrors: v.FieldErrors,
		}

		s.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl.html", data)
		return
	}

	err = s.UserService.CreateUser(&newUser)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			v.AddFieldError("email", "Email address is already in use")

			data := userSignUpData{
				User:        newUser,
				FieldErrors: v.FieldErrors,
			}

			s.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl.html", data)
		} else {
			s.serverError(w, r, err)
		}
		return
	}

	s.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please log in.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

type userLoginData struct {
	User           models.UserInput
	FieldErrors    map[string]string
	NonFieldErrors []string
}

func (s *Server) handleUserLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "login.tmpl.html", userLoginData{})
}

func (s *Server) handleUserLoginPost(w http.ResponseWriter, r *http.Request) {
	var potentialUser models.UserInput

	err := s.decodePostForm(r, &potentialUser)
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	var v validator.Validator

	v.Check(validator.NotBlank(potentialUser.Email), "email", "This field cannot be blank")
	v.Check(validator.Matches(potentialUser.Email, validator.EmailRX), "email", "This field must be a valid email address")
	v.Check(validator.NotBlank(potentialUser.Password), "password", "This field cannot be blank")

	id, err := s.UserService.Authenticate(potentialUser.Email, potentialUser.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			v.AddNonFieldError("Email or password is incorrect")
			data := userLoginData{
				User:           potentialUser,
				FieldErrors:    v.FieldErrors,
				NonFieldErrors: v.NonFieldErrors,
			}
			s.render(w, r, http.StatusUnprocessableEntity, "login.tmpl.html", data)
		} else {
			s.serverError(w, r, err)
		}
		return
	}

	err = s.sessionManager.RenewToken(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.sessionManager.Put(r.Context(), "authenticatedUserID", id)
	http.Redirect(w, r, "/feed/create", http.StatusSeeOther)
}

func (s *Server) handleUserLogoutPost(w http.ResponseWriter, r *http.Request) {
	err := s.sessionManager.RenewToken(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.sessionManager.Remove(r.Context(), "authenticatedUserID")

	s.sessionManager.Put(r.Context(), "flash", "You've been logged out successfully!")

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
