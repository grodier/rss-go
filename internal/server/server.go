package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	"github.com/grodier/rss-go/internal/discovery"
	"github.com/grodier/rss-go/internal/models"
	"github.com/grodier/rss-go/internal/tmpl"
	"github.com/grodier/rss-go/internal/validator"
	"github.com/justinas/nosurf"
)

type Server struct {
	Port int
	Env  string

	FeedService      models.FeedService
	UserService      models.UserService
	DiscoveryStore   models.DiscoveryService
	DiscoveryService *discovery.Service

	template       *tmpl.Template
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

// getUserID retrieves the authenticated user's ID from the session
func (s *Server) getUserID(r *http.Request) (int, error) {
	userID := s.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if userID == 0 {
		return 0, errors.New("not authenticated")
	}
	return userID, nil
}

func (s *Server) handleRootView(w http.ResponseWriter, r *http.Request) {
	// Render different templates based on authentication status
	if s.isAuthenticated(r) {
		// Authenticated users see the dashboard with their subscriptions
		userID, err := s.getUserID(r)
		if err != nil {
			s.serverError(w, r, err)
			return
		}

		feeds, err := s.FeedService.GetUserFeeds(userID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}

		data := struct {
			Feeds           []*models.Feed
			IsAuthenticated bool
			CSRFToken       string
			Flash           string
		}{
			Feeds:           feeds,
			IsAuthenticated: true,
			CSRFToken:       nosurf.Token(r),
		}

		s.render(w, r, http.StatusOK, "dashboard.tmpl.html", data)
	} else {
		// Unauthenticated users see the landing page
		data := struct {
			IsAuthenticated bool
			CSRFToken       string
			Flash           string
		}{
			IsAuthenticated: false,
			CSRFToken:       nosurf.Token(r),
		}

		s.render(w, r, http.StatusOK, "root.tmpl.html", data)
	}
}

func (s *Server) handleAboutView(w http.ResponseWriter, r *http.Request) {
	data := struct {
		IsAuthenticated bool
		CSRFToken       string
		Flash           string
	}{
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
	}

	s.render(w, r, http.StatusOK, "about.tmpl.html", data)
}

func (s *Server) handleFeedsList(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserID(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	feeds, err := s.FeedService.GetUserFeeds(userID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	data := struct {
		Feeds           []*models.Feed
		IsAuthenticated bool
		CSRFToken       string
		Flash           string
	}{
		Feeds:           feeds,
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
		Flash:           s.sessionManager.PopString(r.Context(), "flash"),
	}

	s.render(w, r, http.StatusOK, "feeds_list.tmpl.html", data)
}

type feedViewData struct {
	Feed            models.Feed
	Flash           string
	IsAuthenticated bool
	CSRFToken       string
}

func (s *Server) handleFeedView(w http.ResponseWriter, r *http.Request) {
	id, err := s.readIDParam(r)

	// in future want to return a better msg/page to user
	// and wrap the functionality in utility of some sort
	if err != nil {
		http.NotFound(w, r)
		return
	}

	userID, err := s.getUserID(r)
	if err != nil {
		s.serverError(w, r, err)
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

	// Check if user is subscribed to this feed
	subscribed, err := s.FeedService.IsUserSubscribed(userID, feed.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	if !subscribed {
		http.NotFound(w, r)
		return
	}

	data := feedViewData{
		Feed:            *feed,
		Flash:           s.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
	}

	s.render(w, r, http.StatusOK, "feed.tmpl.html", data)
}

type feedCreateData struct {
	Feed            models.Feed
	FieldErrors     map[string]string
	IsAuthenticated bool
	CSRFToken       string
	Flash           string
}

func (s *Server) handleFeedCreate(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "feed_create.tmpl.html", feedCreateData{
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
	})
}

// TODO: consider moving validation to feed creation and reduce handler responsibilities
func (s *Server) handleFeedCreatePost(w http.ResponseWriter, r *http.Request) {
	var newFeed models.Feed

	err := s.decodePostForm(r, &newFeed)
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	userID, err := s.getUserID(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	var v validator.Validator

	v.Check(validator.NotBlank(newFeed.FeedURL), "feed_url", "This field cannot be blank")

	if !v.Valid() {
		data := feedCreateData{
			Feed:            newFeed,
			FieldErrors:     v.FieldErrors,
			IsAuthenticated: s.isAuthenticated(r),
			CSRFToken:       nosurf.Token(r),
		}

		s.render(w, r, http.StatusUnprocessableEntity, "feed_create.tmpl.html", data)
		return
	}

	// Subscribe user to the feed (creates feed if it doesn't exist)
	feed, err := s.FeedService.SubscribeToFeed(userID, newFeed.FeedURL)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.sessionManager.Put(r.Context(), "flash", "Successfully subscribed to feed")

	http.Redirect(w, r, fmt.Sprintf("/feeds/%d", feed.ID), http.StatusSeeOther)
}

func (s *Server) handleFeedSubscribe(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserID(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	feedURL := r.PostFormValue("feed_url")
	if feedURL == "" {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	feed, err := s.FeedService.SubscribeToFeed(userID, feedURL)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.sessionManager.Put(r.Context(), "flash", "Successfully subscribed to feed")

	// Check if request wants JSON (for AJAX)
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"feed_id":  feed.ID,
			"message":  "Successfully subscribed to feed",
		})
		return
	}

	http.Redirect(w, r, "/feeds", http.StatusSeeOther)
}

func (s *Server) handleFeedUnsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, err := s.getUserID(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	id, err := s.readIDParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	err = s.FeedService.UnsubscribeFromFeed(userID, int(id))
	if err != nil {
		s.serverError(w, r, err)
		return
	}

	s.sessionManager.Put(r.Context(), "flash", "Successfully unsubscribed from feed")

	// Check if request wants JSON (for AJAX)
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Successfully unsubscribed from feed",
		})
		return
	}

	http.Redirect(w, r, "/feeds", http.StatusSeeOther)
}

type userSignUpData struct {
	User            models.UserInput
	FieldErrors     map[string]string
	IsAuthenticated bool
	CSRFToken       string
	Flash           string
}

func (s *Server) handleUserSignUp(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "signup.tmpl.html", userSignUpData{
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
	})
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
			User:            newUser,
			FieldErrors:     v.FieldErrors,
			IsAuthenticated: s.isAuthenticated(r),
			CSRFToken:       nosurf.Token(r),
		}

		s.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl.html", data)
		return
	}

	err = s.UserService.CreateUser(&newUser)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			v.AddFieldError("email", "Email address is already in use")

			data := userSignUpData{
				User:            newUser,
				FieldErrors:     v.FieldErrors,
				IsAuthenticated: s.isAuthenticated(r),
				CSRFToken:       nosurf.Token(r),
			}

			s.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl.html", data)
		} else {
			s.serverError(w, r, err)
		}
		return
	}

	s.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please log in.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type userLoginData struct {
	User            models.UserInput
	FieldErrors     map[string]string
	NonFieldErrors  []string
	IsAuthenticated bool
	CSRFToken       string
	Flash           string
	Redirect        string
}

func (s *Server) handleUserLogin(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Query().Get("redirect")

	s.render(w, r, http.StatusOK, "login.tmpl.html", userLoginData{
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
		Flash:           s.sessionManager.PopString(r.Context(), "flash"),
		Redirect:        redirect,
	})
}

func (s *Server) handleUserLoginPost(w http.ResponseWriter, r *http.Request) {
	var potentialUser models.UserInput

	err := s.decodePostForm(r, &potentialUser)
	if err != nil {
		s.clientError(w, http.StatusBadRequest)
		return
	}

	redirect := r.PostForm.Get("redirect")

	var v validator.Validator

	v.Check(validator.NotBlank(potentialUser.Email), "email", "This field cannot be blank")
	v.Check(validator.Matches(potentialUser.Email, validator.EmailRX), "email", "This field must be a valid email address")
	v.Check(validator.NotBlank(potentialUser.Password), "password", "This field cannot be blank")

	id, err := s.UserService.Authenticate(potentialUser.Email, potentialUser.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			v.AddNonFieldError("Email or password is incorrect")
			data := userLoginData{
				User:            potentialUser,
				FieldErrors:     v.FieldErrors,
				NonFieldErrors:  v.NonFieldErrors,
				IsAuthenticated: s.isAuthenticated(r),
				CSRFToken:       nosurf.Token(r),
				Redirect:        redirect,
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

	// Redirect to the original destination or home if none specified
	if redirect != "" && s.isSafeRedirect(redirect) {
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
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

// Feed search and discovery handlers

type feedSearchData struct {
	Query           string
	Results         []models.FeedCandidate
	Discovery       *models.Discovery
	IsAuthenticated bool
	CSRFToken       string
	Flash           string
}

func (s *Server) handleFeedSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	discoveryID := r.URL.Query().Get("d")

	data := feedSearchData{
		Query:           query,
		Results:         []models.FeedCandidate{},
		IsAuthenticated: s.isAuthenticated(r),
		CSRFToken:       nosurf.Token(r),
		Flash:           s.sessionManager.PopString(r.Context(), "flash"),
	}

	// If query is provided, search known feeds
	if query != "" {
		queryNorm := models.NormalizeQuery(query)
		data.Results = s.DiscoveryStore.SearchKnown(queryNorm)
	}

	// If discovery ID is provided, include discovery state
	if discoveryID != "" {
		if discovery, ok := s.DiscoveryStore.GetDiscovery(discoveryID); ok {
			data.Discovery = discovery
			// Merge discovery results with known results
			if len(discovery.Results) > 0 {
				data.Results = append(data.Results, discovery.Results...)
			}
		}
	}

	s.render(w, r, http.StatusOK, "feed_search.tmpl.html", data)
}

func (s *Server) handleFeedSearchPost(w http.ResponseWriter, r *http.Request) {
	s.logger.Info("received search POST request",
		"method", r.Method,
		"content_type", r.Header.Get("Content-Type"),
		"accept", r.Header.Get("Accept"),
	)

	query := r.FormValue("q")

	s.logger.Info("processing search query",
		"query", query,
		"is_empty", query == "",
	)

	if query == "" {
		s.logger.Warn("search query is empty")
		s.clientError(w, http.StatusBadRequest)
		return
	}

	queryNorm := models.NormalizeQuery(query)
	s.logger.Info("normalized search query",
		"original", query,
		"normalized", queryNorm,
	)

	// Search known feeds
	knownResults := s.DiscoveryStore.SearchKnown(queryNorm)
	s.logger.Info("searched known feeds",
		"query_norm", queryNorm,
		"results_count", len(knownResults),
	)

	// Get user key for rate limiting (session ID)
	userKey := s.sessionManager.Token(r.Context())
	s.logger.Info("got user key for rate limiting",
		"user_key", userKey,
	)

	// Determine if discovery should be triggered
	shouldDiscover := s.DiscoveryService.ShouldDiscover(userKey, queryNorm, knownResults)
	s.logger.Info("checked if discovery should be triggered",
		"should_discover", shouldDiscover,
		"known_results_count", len(knownResults),
		"is_url_like", models.IsURLLike(queryNorm),
	)

	var discoveryID string

	if shouldDiscover {
		s.logger.Info("starting discovery",
			"user_key", userKey,
			"query_norm", queryNorm,
			"query_raw", query,
		)

		// Start discovery
		disc, err := s.DiscoveryService.StartDiscovery(r.Context(), userKey, queryNorm, query)
		if err != nil {
			s.logger.Error("failed to start discovery",
				"query", query,
				"error", err,
			)
		} else {
			discoveryID = disc.ID
			s.logger.Info("discovery started successfully",
				"discovery_id", discoveryID,
				"status", disc.Status,
			)
		}
	} else {
		s.logger.Info("skipping discovery - conditions not met")
	}

	// Check if request wants JSON (for AJAX)
	if r.Header.Get("Accept") == "application/json" {
		s.logger.Info("returning JSON response")
		response := struct {
			Results     []models.FeedCandidate `json:"results"`
			DiscoveryID string                 `json:"discovery_id,omitempty"`
		}{
			Results:     knownResults,
			DiscoveryID: discoveryID,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Redirect to search page with query and discovery ID
	redirectURL := "/feeds/search?q=" + query
	if discoveryID != "" {
		redirectURL += "&d=" + discoveryID
	}

	s.logger.Info("redirecting to search page",
		"redirect_url", redirectURL,
	)

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleFeedSuggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]models.FeedCandidate{})
		return
	}

	suggestions := s.DiscoveryStore.Suggest(query)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(suggestions)
}

func (s *Server) handleFeedDiscoveryEvents(w http.ResponseWriter, r *http.Request) {
	// Get discovery ID from URL path
	discoveryID := r.PathValue("id")

	s.logger.Info("SSE connection request",
		"discovery_id", discoveryID,
		"remote_addr", r.RemoteAddr,
	)

	if discoveryID == "" {
		s.logger.Warn("SSE request missing discovery ID")
		http.NotFound(w, r)
		return
	}

	// Get last event ID for reconnection
	lastEventID := r.Header.Get("Last-Event-ID")
	fromSeq := int64(0)
	if lastEventID != "" {
		if seq, err := strconv.ParseInt(lastEventID, 10, 64); err == nil {
			fromSeq = seq
		}
	}

	s.logger.Info("SSE connection established",
		"discovery_id", discoveryID,
		"from_seq", fromSeq,
	)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error("streaming not supported")
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial events
	events := s.DiscoveryStore.GetDiscoveryEvents(discoveryID, fromSeq)
	s.logger.Info("sending initial SSE events",
		"discovery_id", discoveryID,
		"event_count", len(events),
	)

	for _, evt := range events {
		s.logger.Info("sending SSE event",
			"discovery_id", discoveryID,
			"event_type", evt.Type,
			"event_seq", evt.Seq,
			"message", evt.Message,
		)
		s.sendSSEEvent(w, evt)
		flusher.Flush()
	}

	// Poll for new events
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute) // 2-minute timeout

	s.logger.Info("starting SSE polling loop", "discovery_id", discoveryID)

	for {
		select {
		case <-r.Context().Done():
			s.logger.Info("SSE client disconnected", "discovery_id", discoveryID)
			return
		case <-timeout:
			s.logger.Info("SSE timeout reached", "discovery_id", discoveryID)
			return
		case <-ticker.C:
			// Get discovery state
			discovery, ok := s.DiscoveryStore.GetDiscovery(discoveryID)
			if !ok {
				s.logger.Warn("discovery not found during SSE polling", "discovery_id", discoveryID)
				return
			}

			// Get new events
			newEvents := s.DiscoveryStore.GetDiscoveryEvents(discoveryID, fromSeq)
			if len(newEvents) > 0 {
				s.logger.Info("sending new SSE events",
					"discovery_id", discoveryID,
					"event_count", len(newEvents),
					"discovery_status", discovery.Status,
				)

				for _, evt := range newEvents {
					s.logger.Info("sending SSE event",
						"discovery_id", discoveryID,
						"event_type", evt.Type,
						"event_seq", evt.Seq,
						"message", evt.Message,
					)
					s.sendSSEEvent(w, evt)
					flusher.Flush()
					fromSeq = evt.Seq
				}
			}

			// Send keepalive comment
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()

			// Exit if discovery is complete
			if discovery.Status != "pending" {
				s.logger.Info("discovery complete, closing SSE",
					"discovery_id", discoveryID,
					"status", discovery.Status,
				)
				return
			}
		}
	}
}

func (s *Server) sendSSEEvent(w http.ResponseWriter, evt models.DiscoveryEvent) {
	// Send event ID for reconnection
	fmt.Fprintf(w, "id: %d\n", evt.Seq)

	// Send event type
	if evt.Type != "" {
		fmt.Fprintf(w, "event: %s\n", evt.Type)
	}

	// Send event data
	data := struct {
		Message string                 `json:"message"`
		Results []models.FeedCandidate `json:"results,omitempty"`
	}{
		Message: evt.Message,
		Results: evt.Results,
	}

	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
}
