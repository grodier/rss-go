package discovery

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/grodier/rss-go/internal/models"
	"github.com/grodier/rss-go/internal/queue"
)

// mockDiscoveryStore is a mock implementation of models.DiscoveryService for testing
type mockDiscoveryStore struct {
	knownFeeds   []models.FeedCandidate
	discoveries  map[string]*models.Discovery
	discoveryIdx map[string]string
	events       map[string][]models.DiscoveryEvent
}

func newMockDiscoveryStore() *mockDiscoveryStore {
	return &mockDiscoveryStore{
		knownFeeds:   make([]models.FeedCandidate, 0),
		discoveries:  make(map[string]*models.Discovery),
		discoveryIdx: make(map[string]string),
		events:       make(map[string][]models.DiscoveryEvent),
	}
}

func (m *mockDiscoveryStore) SearchKnown(queryNorm string) []models.FeedCandidate {
	return m.knownFeeds
}

func (m *mockDiscoveryStore) Suggest(partial string) []models.FeedCandidate {
	return []models.FeedCandidate{}
}

func (m *mockDiscoveryStore) CreateOrGetDiscovery(userKey, queryNorm, queryRaw string) (*models.Discovery, bool) {
	key := userKey + "|" + queryNorm
	if existingID, exists := m.discoveryIdx[key]; exists {
		return m.discoveries[existingID], false
	}

	discovery := &models.Discovery{
		ID:        "test-discovery-" + queryNorm,
		UserKey:   userKey,
		QueryRaw:  queryRaw,
		QueryNorm: queryNorm,
		Status:    "pending",
		Results:   make([]models.FeedCandidate, 0),
		Events:    make([]models.DiscoveryEvent, 0),
		Seq:       0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.discoveries[discovery.ID] = discovery
	m.discoveryIdx[key] = discovery.ID
	m.events[discovery.ID] = make([]models.DiscoveryEvent, 0)

	return discovery, true
}

func (m *mockDiscoveryStore) GetDiscovery(id string) (*models.Discovery, bool) {
	discovery, ok := m.discoveries[id]
	return discovery, ok
}

func (m *mockDiscoveryStore) GetDiscoveryByUserAndQuery(userKey, queryNorm string) (*models.Discovery, bool) {
	key := userKey + "|" + queryNorm
	if existingID, exists := m.discoveryIdx[key]; exists {
		return m.discoveries[existingID], true
	}
	return nil, false
}

func (m *mockDiscoveryStore) AppendDiscoveryEvent(id string, evt models.DiscoveryEvent) {
	if _, ok := m.events[id]; !ok {
		m.events[id] = make([]models.DiscoveryEvent, 0)
	}
	m.events[id] = append(m.events[id], evt)

	if discovery, ok := m.discoveries[id]; ok {
		discovery.Events = append(discovery.Events, evt)
	}
}

func (m *mockDiscoveryStore) UpdateDiscovery(id string, fn func(*models.Discovery)) {
	if discovery, ok := m.discoveries[id]; ok {
		fn(discovery)
		discovery.UpdatedAt = time.Now()
	}
}

func (m *mockDiscoveryStore) AddKnownFeed(feed models.FeedCandidate) {
	m.knownFeeds = append(m.knownFeeds, feed)
}

func (m *mockDiscoveryStore) GetDiscoveryEvents(id string, fromSeq int64) []models.DiscoveryEvent {
	events, ok := m.events[id]
	if !ok {
		return []models.DiscoveryEvent{}
	}

	filtered := make([]models.DiscoveryEvent, 0)
	for _, evt := range events {
		if evt.Seq > fromSeq {
			filtered = append(filtered, evt)
		}
	}
	return filtered
}

func TestShouldDiscover(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name         string
		queryNorm    string
		knownResults []models.FeedCandidate
		existing     *models.Discovery
		expected     bool
	}{
		{
			name:         "should discover - URL-like query with no known results",
			queryNorm:    "example.com",
			knownResults: []models.FeedCandidate{},
			expected:     true,
		},
		{
			name:      "should not discover - has known results",
			queryNorm: "example.com",
			knownResults: []models.FeedCandidate{
				{Title: "Example Feed", FeedURL: "https://example.com/feed"},
			},
			expected: false,
		},
		{
			name:         "should not discover - not URL-like",
			queryNorm:    "tech news",
			knownResults: []models.FeedCandidate{},
			expected:     false,
		},
		{
			name:      "should not discover - discovery in progress",
			queryNorm: "example.com",
			existing: &models.Discovery{
				ID:        "test-1",
				Status:    "pending",
				QueryNorm: "example.com",
			},
			knownResults: []models.FeedCandidate{},
			expected:     false,
		},
		{
			name:      "should discover - previous discovery completed",
			queryNorm: "example.com",
			existing: &models.Discovery{
				ID:        "test-1",
				Status:    "resolved_found",
				QueryNorm: "example.com",
			},
			knownResults: []models.FeedCandidate{},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockDiscoveryStore()
			q := queue.NewInMemQueue(1, logger)
			service := NewService(store, q, logger)

			if tt.existing != nil {
				store.discoveries[tt.existing.ID] = tt.existing
				store.discoveryIdx["user1|"+tt.queryNorm] = tt.existing.ID
			}

			result := service.ShouldDiscover("user1", tt.queryNorm, tt.knownResults)

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStartDiscovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("create new discovery", func(t *testing.T) {
		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)

		discovery, err := service.StartDiscovery(context.Background(), "user1", "example.com", "https://example.com")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if discovery.Status != "pending" {
			t.Errorf("expected status 'pending', got %q", discovery.Status)
		}

		if discovery.QueryNorm != "example.com" {
			t.Errorf("expected query norm 'example.com', got %q", discovery.QueryNorm)
		}

		// Verify event was emitted
		if len(discovery.Events) == 0 {
			t.Error("expected at least one event")
		}
	})

	t.Run("return existing pending discovery", func(t *testing.T) {
		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)

		// Create first discovery
		discovery1, _ := service.StartDiscovery(context.Background(), "user1", "example.com", "https://example.com")

		// Try to create same discovery again
		discovery2, err := service.StartDiscovery(context.Background(), "user1", "example.com", "https://example.com")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if discovery1.ID != discovery2.ID {
			t.Errorf("expected same discovery ID, got %q and %q", discovery1.ID, discovery2.ID)
		}
	})

	t.Run("reset completed discovery", func(t *testing.T) {
		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)

		// Create and complete a discovery
		discovery, _ := service.StartDiscovery(context.Background(), "user1", "example.com", "https://example.com")
		store.UpdateDiscovery(discovery.ID, func(d *models.Discovery) {
			d.Status = "resolved_found"
			d.Message = "Found feeds"
			d.Results = []models.FeedCandidate{{Title: "Test"}}
		})

		// Start discovery again
		discovery2, err := service.StartDiscovery(context.Background(), "user1", "example.com", "https://example.com")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if discovery2.Status != "pending" {
			t.Errorf("expected status to be reset to 'pending', got %q", discovery2.Status)
		}

		if discovery2.Message != "" {
			t.Errorf("expected message to be cleared, got %q", discovery2.Message)
		}

		if len(discovery2.Results) != 0 {
			t.Errorf("expected results to be cleared, got %d results", len(discovery2.Results))
		}
	})
}

func TestDiscoveryJob_ParseRSS(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	store := newMockDiscoveryStore()
	q := queue.NewInMemQueue(1, logger)
	service := NewService(store, q, logger)

	job := &DiscoveryJob{
		DiscoveryID: "test-1",
		Query:       "example.com",
		service:     service,
	}

	rssXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example Feed</title>
    <link>https://example.com</link>
    <description>An example RSS feed</description>
  </channel>
</rss>`)

	feed := job.parseRSS(rssXML)

	if feed == nil {
		t.Fatal("expected feed to be parsed")
	}

	if feed.Title != "Example Feed" {
		t.Errorf("expected title 'Example Feed', got %q", feed.Title)
	}

	if feed.SiteURL != "https://example.com" {
		t.Errorf("expected site URL 'https://example.com', got %q", feed.SiteURL)
	}

	if feed.Reason != "RSS 2.0 feed" {
		t.Errorf("expected reason 'RSS 2.0 feed', got %q", feed.Reason)
	}
}

func TestDiscoveryJob_ParseAtom(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	store := newMockDiscoveryStore()
	q := queue.NewInMemQueue(1, logger)
	service := NewService(store, q, logger)

	job := &DiscoveryJob{
		DiscoveryID: "test-1",
		Query:       "example.com",
		service:     service,
	}

	atomXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom Feed</title>
  <link href="https://example.com" rel="alternate"/>
  <updated>2023-01-01T00:00:00Z</updated>
</feed>`)

	feed := job.parseAtom(atomXML)

	if feed == nil {
		t.Fatal("expected feed to be parsed")
	}

	if feed.Title != "Example Atom Feed" {
		t.Errorf("expected title 'Example Atom Feed', got %q", feed.Title)
	}

	if feed.SiteURL != "https://example.com" {
		t.Errorf("expected site URL 'https://example.com', got %q", feed.SiteURL)
	}

	if feed.Reason != "Atom feed" {
		t.Errorf("expected reason 'Atom feed', got %q", feed.Reason)
	}
}

func TestDiscoveryJob_Run(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("successful RSS discovery", func(t *testing.T) {
		// Create a test server that returns RSS
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>https://example.com</link>
    <description>Test description</description>
  </channel>
</rss>`))
		}))
		defer ts.Close()

		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)
		service.allowPrivateIPs = true // Allow localhost for testing

		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", ts.URL)

		job := &DiscoveryJob{
			DiscoveryID: discovery.ID,
			Query:       ts.URL,
			service:     service,
		}

		err := job.Run(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify discovery was updated
		updated, _ := store.GetDiscovery(discovery.ID)
		if updated.Status != "resolved_found" {
			t.Errorf("expected status 'resolved_found', got %q", updated.Status)
		}

		if len(updated.Results) != 1 {
			t.Errorf("expected 1 result, got %d", len(updated.Results))
		}

		if updated.Results[0].Title != "Test Feed" {
			t.Errorf("expected title 'Test Feed', got %q", updated.Results[0].Title)
		}

		// Verify feed was added to known feeds
		if len(store.knownFeeds) != 1 {
			t.Errorf("expected 1 known feed, got %d", len(store.knownFeeds))
		}
	})

	t.Run("no feeds found", func(t *testing.T) {
		// Create a test server that returns HTML (not a feed)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body>Not a feed</body></html>`))
		}))
		defer ts.Close()

		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)
		service.allowPrivateIPs = true // Allow localhost for testing

		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", ts.URL)

		job := &DiscoveryJob{
			DiscoveryID: discovery.ID,
			Query:       ts.URL,
			service:     service,
		}

		err := job.Run(context.Background())

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify discovery was updated
		updated, _ := store.GetDiscovery(discovery.ID)
		if updated.Status != "resolved_none" {
			t.Errorf("expected status 'resolved_none', got %q", updated.Status)
		}

		if len(updated.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(updated.Results))
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		// Create a test server that returns 404
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		store := newMockDiscoveryStore()
		q := queue.NewInMemQueue(10, logger)
		service := NewService(store, q, logger)
		service.allowPrivateIPs = true // Allow localhost for testing

		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", ts.URL)

		job := &DiscoveryJob{
			DiscoveryID: discovery.ID,
			Query:       ts.URL,
			service:     service,
		}

		err := job.Run(context.Background())

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Verify discovery was marked as error
		updated, _ := store.GetDiscovery(discovery.ID)
		if updated.Status != "error" {
			t.Errorf("expected status 'error', got %q", updated.Status)
		}
	})
}

func TestDiscoveryJob_NormalizeURL(t *testing.T) {
	job := &DiscoveryJob{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "add https to domain",
			input:    "example.com",
			expected: "https://example.com",
		},
		{
			name:     "preserve existing https",
			input:    "https://example.com",
			expected: "https://example.com",
		},
		{
			name:     "preserve existing http",
			input:    "http://example.com",
			expected: "http://example.com",
		},
		{
			name:     "trim whitespace and add https",
			input:    "  example.com  ",
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := job.normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		{
			name:     "localhost",
			hostname: "localhost",
			expected: true,
		},
		{
			name:     "127.0.0.1",
			hostname: "127.0.0.1",
			expected: true,
		},
		{
			name:     "0.0.0.0",
			hostname: "0.0.0.0",
			expected: true,
		},
		{
			name:     "IPv6 localhost",
			hostname: "::1",
			expected: true,
		},
		{
			name:     "10.x.x.x",
			hostname: "10.0.0.1",
			expected: true,
		},
		{
			name:     "172.16.x.x",
			hostname: "172.16.0.1",
			expected: true,
		},
		{
			name:     "192.168.x.x",
			hostname: "192.168.1.1",
			expected: true,
		},
		{
			name:     "169.254.x.x",
			hostname: "169.254.0.1",
			expected: true,
		},
		{
			name:     "public IP",
			hostname: "8.8.8.8",
			expected: false,
		},
		{
			name:     "public domain",
			hostname: "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateIP(tt.hostname)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDiscoveryJob_Kind(t *testing.T) {
	job := &DiscoveryJob{DiscoveryID: "test-1", Query: "example.com"}
	if job.Kind() != "discovery" {
		t.Errorf("expected kind 'discovery', got %q", job.Kind())
	}
}

func TestDiscoveryJob_Key(t *testing.T) {
	job := &DiscoveryJob{DiscoveryID: "test-123", Query: "example.com"}
	expected := "discovery:test-123"
	if job.Key() != expected {
		t.Errorf("expected key %q, got %q", expected, job.Key())
	}
}
