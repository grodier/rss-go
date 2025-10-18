package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/template"
	"time"

	"github.com/julienschmidt/httprouter"
)

type Server struct {
	Port int
	Env  string

	server *http.Server
	logger *slog.Logger
}

func NewServer(logger *slog.Logger) *Server {
	s := &Server{
		server: &http.Server{},
		logger: logger,
	}

	return s
}

func (s *Server) Serve() error {
	s.server.Handler = s.routes()
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

func (s *Server) routes() http.Handler {
	router := httprouter.New()

	router.HandlerFunc(http.MethodGet, "/", s.handleRootView)
	router.HandlerFunc(http.MethodGet, "/api/v1/healthcheck", s.handleHealthcheck)

	return router
}

func (s *Server) handleRootView(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")

	// Use the template.ParseFiles() function to read the template file into a
	// template set. If there's an error, we log the detailed error message, use
	// the http.Error() function to send an Internal Server Error response to the
	// user, and then return from the handler so no subsequent code is executed.
	ts, err := template.ParseFiles("./internal/html/pages/root.tmpl.html")
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Then we use the Execute() method on the template set to write the
	// template content as the response body. The last parameter to Execute()
	// represents any dynamic data that we want to pass in, which for now we'll
	// leave as nil.
	err = ts.Execute(w, nil)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (s *Server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
