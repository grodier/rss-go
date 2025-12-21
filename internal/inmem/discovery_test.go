package inmem

import (
	"testing"

	"github.com/grodier/rss-go/internal/models"
)

func TestSearchKnown(t *testing.T) {
	tests := []struct {
		name          string
		feeds         []models.FeedCandidate
		query         string
		expectedCount int
		expectedTitle string
	}{
		{
			name: "search by title",
			feeds: []models.FeedCandidate{
				{
					ID:      "1",
					Title:   "Tech News",
					FeedURL: "https://example.com/feed",
					SiteURL: "https://example.com",
					Source:  "known",
				},
			},
			query:         "tech",
			expectedCount: 1,
			expectedTitle: "Tech News",
		},
		{
			name: "search by feed URL",
			feeds: []models.FeedCandidate{
				{
					ID:      "1",
					Title:   "My Blog",
					FeedURL: "https://myblog.com/rss",
					SiteURL: "https://myblog.com",
					Source:  "known",
				},
			},
			query:         "myblog.com",
			expectedCount: 1,
			expectedTitle: "My Blog",
		},
		{
			name: "search by site URL",
			feeds: []models.FeedCandidate{
				{
					ID:      "1",
					Title:   "Example Site",
					FeedURL: "https://feeds.example.net/rss",
					SiteURL: "https://example.net",
					Source:  "known",
				},
			},
			query:         "example.net",
			expectedCount: 1,
			expectedTitle: "Example Site",
		},
		{
			name: "case insensitive search",
			feeds: []models.FeedCandidate{
				{
					ID:      "1",
					Title:   "Tech News",
					FeedURL: "https://example.com/feed",
					SiteURL: "https://example.com",
					Source:  "known",
				},
			},
			query:         "TECH",
			expectedCount: 1,
			expectedTitle: "Tech News",
		},
		{
			name: "no matches",
			feeds: []models.FeedCandidate{
				{
					ID:      "1",
					Title:   "Tech News",
					FeedURL: "https://example.com/feed",
					SiteURL: "https://example.com",
					Source:  "known",
				},
			},
			query:         "nonexistent",
			expectedCount: 0,
		},
		{
			name:          "empty query",
			feeds:         []models.FeedCandidate{},
			query:         "",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewDiscoveryStore()
			// Clear seeded feeds
			store.knownFeeds = []models.FeedCandidate{}
			store.feedsByURL = make(map[string]models.FeedCandidate)

			// Add test feeds
			for _, feed := range tt.feeds {
				store.AddKnownFeed(feed)
			}

			results := store.SearchKnown(tt.query)

			if len(results) != tt.expectedCount {
				t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
			}

			if tt.expectedCount > 0 && results[0].Title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, results[0].Title)
			}
		})
	}
}

func TestAddKnownFeed(t *testing.T) {
	t.Run("add new feed", func(t *testing.T) {
		store := NewDiscoveryStore()
		initialCount := len(store.knownFeeds)

		feed := models.FeedCandidate{
			ID:      "test-1",
			Title:   "Test Feed",
			FeedURL: "https://test.com/feed",
			SiteURL: "https://test.com",
			Source:  "known",
		}

		store.AddKnownFeed(feed)

		if len(store.knownFeeds) != initialCount+1 {
			t.Errorf("expected %d feeds, got %d", initialCount+1, len(store.knownFeeds))
		}

		// Verify feed is in both stores
		if _, exists := store.feedsByURL[feed.FeedURL]; !exists {
			t.Error("feed not found in feedsByURL index")
		}
	})

	t.Run("deduplicate by feed URL", func(t *testing.T) {
		store := NewDiscoveryStore()
		store.knownFeeds = []models.FeedCandidate{}
		store.feedsByURL = make(map[string]models.FeedCandidate)

		feed1 := models.FeedCandidate{
			ID:      "test-1",
			Title:   "Test Feed",
			FeedURL: "https://test.com/feed",
			SiteURL: "https://test.com",
			Source:  "known",
		}

		feed2 := models.FeedCandidate{
			ID:      "test-2",
			Title:   "Test Feed Updated",
			FeedURL: "https://test.com/feed", // Same URL
			SiteURL: "https://test.com",
			Source:  "known",
		}

		store.AddKnownFeed(feed1)
		store.AddKnownFeed(feed2)

		if len(store.knownFeeds) != 1 {
			t.Errorf("expected 1 feed after deduplication, got %d", len(store.knownFeeds))
		}

		if len(store.feedsByURL) != 1 {
			t.Errorf("expected 1 feed in index after deduplication, got %d", len(store.feedsByURL))
		}
	})
}

func TestSuggest(t *testing.T) {
	tests := []struct {
		name          string
		partial       string
		expectedCount int
		shouldContain string
	}{
		{
			name:          "prefix match on title",
			partial:       "hack",
			expectedCount: 1,
			shouldContain: "Hacker News",
		},
		{
			name:          "partial too short",
			partial:       "h",
			expectedCount: 0,
		},
		{
			name:          "case insensitive",
			partial:       "HACK",
			expectedCount: 1,
			shouldContain: "Hacker News",
		},
		{
			name:          "URL match",
			partial:       "techcrunch",
			expectedCount: 1,
			shouldContain: "TechCrunch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewDiscoveryStore()
			results := store.Suggest(tt.partial)

			if len(results) < tt.expectedCount {
				t.Errorf("expected at least %d results, got %d", tt.expectedCount, len(results))
			}

			if tt.shouldContain != "" {
				found := false
				for _, result := range results {
					if result.Title == tt.shouldContain {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected results to contain %q", tt.shouldContain)
				}
			}
		})
	}
}

func TestCreateOrGetDiscovery(t *testing.T) {
	t.Run("create new discovery", func(t *testing.T) {
		store := NewDiscoveryStore()

		discovery, isNew := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		if !isNew {
			t.Error("expected isNew to be true for new discovery")
		}

		if discovery.UserKey != "user1" {
			t.Errorf("expected user key 'user1', got %q", discovery.UserKey)
		}

		if discovery.QueryNorm != "example.com" {
			t.Errorf("expected normalized query 'example.com', got %q", discovery.QueryNorm)
		}

		if discovery.QueryRaw != "https://example.com" {
			t.Errorf("expected raw query 'https://example.com', got %q", discovery.QueryRaw)
		}

		if discovery.Status != "pending" {
			t.Errorf("expected status 'pending', got %q", discovery.Status)
		}
	})

	t.Run("get existing discovery", func(t *testing.T) {
		store := NewDiscoveryStore()

		// Create first discovery
		discovery1, isNew1 := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")
		if !isNew1 {
			t.Fatal("expected first discovery to be new")
		}

		// Try to create same discovery again
		discovery2, isNew2 := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		if isNew2 {
			t.Error("expected isNew to be false for existing discovery")
		}

		if discovery1.ID != discovery2.ID {
			t.Errorf("expected same discovery ID, got %q and %q", discovery1.ID, discovery2.ID)
		}
	})

	t.Run("different users can have same query", func(t *testing.T) {
		store := NewDiscoveryStore()

		discovery1, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")
		discovery2, isNew2 := store.CreateOrGetDiscovery("user2", "example.com", "https://example.com")

		if !isNew2 {
			t.Error("expected different user to create new discovery")
		}

		if discovery1.ID == discovery2.ID {
			t.Error("expected different discovery IDs for different users")
		}
	})
}

func TestGetDiscovery(t *testing.T) {
	t.Run("get existing discovery", func(t *testing.T) {
		store := NewDiscoveryStore()
		created, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		retrieved, ok := store.GetDiscovery(created.ID)

		if !ok {
			t.Fatal("expected to find discovery")
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID %q, got %q", created.ID, retrieved.ID)
		}
	})

	t.Run("get non-existent discovery", func(t *testing.T) {
		store := NewDiscoveryStore()

		_, ok := store.GetDiscovery("non-existent-id")

		if ok {
			t.Error("expected not to find discovery")
		}
	})
}

func TestGetDiscoveryByUserAndQuery(t *testing.T) {
	t.Run("find existing discovery", func(t *testing.T) {
		store := NewDiscoveryStore()
		store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		retrieved, ok := store.GetDiscoveryByUserAndQuery("user1", "example.com")

		if !ok {
			t.Fatal("expected to find discovery")
		}

		if retrieved.UserKey != "user1" {
			t.Errorf("expected user key 'user1', got %q", retrieved.UserKey)
		}

		if retrieved.QueryNorm != "example.com" {
			t.Errorf("expected normalized query 'example.com', got %q", retrieved.QueryNorm)
		}
	})

	t.Run("not found for different user", func(t *testing.T) {
		store := NewDiscoveryStore()
		store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		_, ok := store.GetDiscoveryByUserAndQuery("user2", "example.com")

		if ok {
			t.Error("expected not to find discovery for different user")
		}
	})

	t.Run("not found for different query", func(t *testing.T) {
		store := NewDiscoveryStore()
		store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		_, ok := store.GetDiscoveryByUserAndQuery("user1", "other.com")

		if ok {
			t.Error("expected not to find discovery for different query")
		}
	})
}

func TestAppendDiscoveryEvent(t *testing.T) {
	t.Run("append events", func(t *testing.T) {
		store := NewDiscoveryStore()
		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		evt1 := models.DiscoveryEvent{
			Seq:     1,
			Type:    "progress",
			Message: "Starting...",
		}

		evt2 := models.DiscoveryEvent{
			Seq:     2,
			Type:    "done",
			Message: "Complete",
		}

		store.AppendDiscoveryEvent(discovery.ID, evt1)
		store.AppendDiscoveryEvent(discovery.ID, evt2)

		updated, _ := store.GetDiscovery(discovery.ID)

		if len(updated.Events) != 2 {
			t.Errorf("expected 2 events, got %d", len(updated.Events))
		}

		if updated.Events[0].Message != "Starting..." {
			t.Errorf("expected first event message 'Starting...', got %q", updated.Events[0].Message)
		}

		if updated.Events[1].Message != "Complete" {
			t.Errorf("expected second event message 'Complete', got %q", updated.Events[1].Message)
		}
	})

	t.Run("trim old events", func(t *testing.T) {
		store := NewDiscoveryStore()
		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		// Add more than maxEventsPerDiscovery events
		for i := 0; i < maxEventsPerDiscovery+10; i++ {
			evt := models.DiscoveryEvent{
				Seq:     int64(i),
				Type:    "progress",
				Message: "Event",
			}
			store.AppendDiscoveryEvent(discovery.ID, evt)
		}

		updated, _ := store.GetDiscovery(discovery.ID)

		if len(updated.Events) != maxEventsPerDiscovery {
			t.Errorf("expected %d events, got %d", maxEventsPerDiscovery, len(updated.Events))
		}
	})
}

func TestUpdateDiscovery(t *testing.T) {
	t.Run("update discovery status", func(t *testing.T) {
		store := NewDiscoveryStore()
		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		store.UpdateDiscovery(discovery.ID, func(d *models.Discovery) {
			d.Status = "resolved_found"
			d.Message = "Found 2 feeds"
		})

		updated, _ := store.GetDiscovery(discovery.ID)

		if updated.Status != "resolved_found" {
			t.Errorf("expected status 'resolved_found', got %q", updated.Status)
		}

		if updated.Message != "Found 2 feeds" {
			t.Errorf("expected message 'Found 2 feeds', got %q", updated.Message)
		}
	})

	t.Run("update non-existent discovery", func(t *testing.T) {
		store := NewDiscoveryStore()

		// Should not panic
		store.UpdateDiscovery("non-existent", func(d *models.Discovery) {
			d.Status = "error"
		})
	})
}

func TestGetDiscoveryEvents(t *testing.T) {
	t.Run("get all events", func(t *testing.T) {
		store := NewDiscoveryStore()
		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		evt1 := models.DiscoveryEvent{Seq: 1, Type: "progress", Message: "Starting..."}
		evt2 := models.DiscoveryEvent{Seq: 2, Type: "done", Message: "Complete"}

		store.AppendDiscoveryEvent(discovery.ID, evt1)
		store.AppendDiscoveryEvent(discovery.ID, evt2)

		events := store.GetDiscoveryEvents(discovery.ID, 0)

		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("get events from sequence", func(t *testing.T) {
		store := NewDiscoveryStore()
		discovery, _ := store.CreateOrGetDiscovery("user1", "example.com", "https://example.com")

		evt1 := models.DiscoveryEvent{Seq: 1, Type: "progress", Message: "Starting..."}
		evt2 := models.DiscoveryEvent{Seq: 2, Type: "progress", Message: "Fetching..."}
		evt3 := models.DiscoveryEvent{Seq: 3, Type: "done", Message: "Complete"}

		store.AppendDiscoveryEvent(discovery.ID, evt1)
		store.AppendDiscoveryEvent(discovery.ID, evt2)
		store.AppendDiscoveryEvent(discovery.ID, evt3)

		events := store.GetDiscoveryEvents(discovery.ID, 1)

		if len(events) != 2 {
			t.Errorf("expected 2 events after seq 1, got %d", len(events))
		}

		if events[0].Seq != 2 {
			t.Errorf("expected first event seq 2, got %d", events[0].Seq)
		}
	})

	t.Run("get events for non-existent discovery", func(t *testing.T) {
		store := NewDiscoveryStore()

		events := store.GetDiscoveryEvents("non-existent", 0)

		if len(events) != 0 {
			t.Errorf("expected 0 events, got %d", len(events))
		}
	})
}

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase",
			input:    "EXAMPLE.COM",
			expected: "example.com",
		},
		{
			name:     "trim whitespace",
			input:    "  example.com  ",
			expected: "example.com",
		},
		{
			name:     "remove http prefix",
			input:    "http://example.com",
			expected: "example.com",
		},
		{
			name:     "remove https prefix",
			input:    "https://example.com",
			expected: "example.com",
		},
		{
			name:     "remove www prefix",
			input:    "www.example.com",
			expected: "example.com",
		},
		{
			name:     "remove trailing slash",
			input:    "example.com/",
			expected: "example.com",
		},
		{
			name:     "combined normalization",
			input:    "  HTTPS://WWW.EXAMPLE.COM/  ",
			expected: "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.NormalizeQuery(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIsURLLike(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "http URL",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "https URL",
			input:    "https://example.com",
			expected: true,
		},
		{
			name:     "domain with dot",
			input:    "example.com",
			expected: true,
		},
		{
			name:     "subdomain",
			input:    "blog.example.com",
			expected: true,
		},
		{
			name:     "plain text with spaces",
			input:    "tech news",
			expected: false,
		},
		{
			name:     "single word",
			input:    "technology",
			expected: false,
		},
		{
			name:     "text with dot but has spaces",
			input:    "example.com news",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := models.IsURLLike(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
