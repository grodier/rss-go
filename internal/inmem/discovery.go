package inmem

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/grodier/rss-go/internal/models"
)

const (
	maxEventsPerDiscovery = 100 // Limit SSE event history per discovery
)

// DiscoveryStore is an in-memory implementation of DiscoveryService
type DiscoveryStore struct {
	mu           sync.RWMutex
	knownFeeds   []models.FeedCandidate            // Known feeds for search
	feedsByURL   map[string]models.FeedCandidate   // Index by FeedURL for deduplication
	discoveries  map[string]*models.Discovery       // Active discoveries by ID
	discoveryIdx map[string]string                  // (userKey, queryNorm) -> discoveryID
}

// NewDiscoveryStore creates a new in-memory discovery store
func NewDiscoveryStore() *DiscoveryStore {
	store := &DiscoveryStore{
		knownFeeds:   make([]models.FeedCandidate, 0),
		feedsByURL:   make(map[string]models.FeedCandidate),
		discoveries:  make(map[string]*models.Discovery),
		discoveryIdx: make(map[string]string),
	}

	// Seed with some known feeds for testing
	store.seedKnownFeeds()

	return store
}

// seedKnownFeeds adds some well-known RSS feeds for testing
func (s *DiscoveryStore) seedKnownFeeds() {
	knownFeeds := []models.FeedCandidate{
		{
			ID:         generateID(),
			Title:      "Hacker News",
			FeedURL:    "https://news.ycombinator.com/rss",
			SiteURL:    "https://news.ycombinator.com",
			Source:     "known",
			Confidence: 100,
			Reason:     "Well-known tech news aggregator",
		},
		{
			ID:         generateID(),
			Title:      "The Verge",
			FeedURL:    "https://www.theverge.com/rss/index.xml",
			SiteURL:    "https://www.theverge.com",
			Source:     "known",
			Confidence: 100,
			Reason:     "Popular technology news site",
		},
		{
			ID:         generateID(),
			Title:      "TechCrunch",
			FeedURL:    "https://techcrunch.com/feed/",
			SiteURL:    "https://techcrunch.com",
			Source:     "known",
			Confidence: 100,
			Reason:     "Startup and technology news",
		},
		{
			ID:         generateID(),
			Title:      "Ars Technica",
			FeedURL:    "https://feeds.arstechnica.com/arstechnica/index",
			SiteURL:    "https://arstechnica.com",
			Source:     "known",
			Confidence: 100,
			Reason:     "Technology news and information",
		},
		{
			ID:         generateID(),
			Title:      "Wired",
			FeedURL:    "https://www.wired.com/feed/rss",
			SiteURL:    "https://www.wired.com",
			Source:     "known",
			Confidence: 100,
			Reason:     "Technology, science, and culture magazine",
		},
	}

	for _, feed := range knownFeeds {
		s.AddKnownFeed(feed)
	}
}

// SearchKnown searches known feeds by normalized query
func (s *DiscoveryStore) SearchKnown(queryNorm string) []models.FeedCandidate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if queryNorm == "" {
		return []models.FeedCandidate{}
	}

	results := make([]models.FeedCandidate, 0)
	queryLower := strings.ToLower(queryNorm)

	for _, feed := range s.knownFeeds {
		// Search in title, feedURL, and siteURL
		titleMatch := strings.Contains(strings.ToLower(feed.Title), queryLower)
		feedURLMatch := strings.Contains(strings.ToLower(feed.FeedURL), queryLower)
		siteURLMatch := strings.Contains(strings.ToLower(feed.SiteURL), queryLower)

		if titleMatch || feedURLMatch || siteURLMatch {
			results = append(results, feed)
		}
	}

	return results
}

// Suggest returns autocomplete suggestions for partial queries
func (s *DiscoveryStore) Suggest(partial string) []models.FeedCandidate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(partial) < 2 {
		return []models.FeedCandidate{}
	}

	results := make([]models.FeedCandidate, 0)
	partialLower := strings.ToLower(partial)

	for _, feed := range s.knownFeeds {
		// Prefix match on title or URL
		titleMatch := strings.HasPrefix(strings.ToLower(feed.Title), partialLower)
		urlMatch := strings.Contains(strings.ToLower(feed.SiteURL), partialLower)

		if titleMatch || urlMatch {
			results = append(results, feed)
			if len(results) >= 10 {
				break
			}
		}
	}

	return results
}

// CreateOrGetDiscovery creates a new discovery or returns existing one
func (s *DiscoveryStore) CreateOrGetDiscovery(userKey, queryNorm, queryRaw string) (*models.Discovery, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if discovery already exists for this user + query
	idx := discoveryKey(userKey, queryNorm)
	if existingID, exists := s.discoveryIdx[idx]; exists {
		if discovery, ok := s.discoveries[existingID]; ok {
			return discovery, false
		}
	}

	// Create new discovery
	discovery := &models.Discovery{
		ID:        generateID(),
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

	s.discoveries[discovery.ID] = discovery
	s.discoveryIdx[idx] = discovery.ID

	return discovery, true
}

// GetDiscovery retrieves a discovery by ID
func (s *DiscoveryStore) GetDiscovery(id string) (*models.Discovery, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	discovery, ok := s.discoveries[id]
	return discovery, ok
}

// GetDiscoveryByUserAndQuery checks if a discovery exists for a user+query without creating it
func (s *DiscoveryStore) GetDiscoveryByUserAndQuery(userKey, queryNorm string) (*models.Discovery, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	idx := discoveryKey(userKey, queryNorm)
	if existingID, exists := s.discoveryIdx[idx]; exists {
		if discovery, ok := s.discoveries[existingID]; ok {
			return discovery, true
		}
	}

	return nil, false
}

// AppendDiscoveryEvent adds an event to a discovery's event stream
func (s *DiscoveryStore) AppendDiscoveryEvent(id string, evt models.DiscoveryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	discovery, ok := s.discoveries[id]
	if !ok {
		return
	}

	// Append event
	discovery.Events = append(discovery.Events, evt)

	// Trim old events if we exceed the limit
	if len(discovery.Events) > maxEventsPerDiscovery {
		discovery.Events = discovery.Events[len(discovery.Events)-maxEventsPerDiscovery:]
	}

	discovery.UpdatedAt = time.Now()
}

// UpdateDiscovery atomically updates a discovery
func (s *DiscoveryStore) UpdateDiscovery(id string, fn func(*models.Discovery)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	discovery, ok := s.discoveries[id]
	if !ok {
		return
	}

	fn(discovery)
	discovery.UpdatedAt = time.Now()
}

// AddKnownFeed adds a feed to the known feeds index
func (s *DiscoveryStore) AddKnownFeed(feed models.FeedCandidate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deduplicate by FeedURL
	if _, exists := s.feedsByURL[feed.FeedURL]; exists {
		return
	}

	s.feedsByURL[feed.FeedURL] = feed
	s.knownFeeds = append(s.knownFeeds, feed)
}

// GetDiscoveryEvents returns events for SSE streaming, optionally from a sequence
func (s *DiscoveryStore) GetDiscoveryEvents(id string, fromSeq int64) []models.DiscoveryEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	discovery, ok := s.discoveries[id]
	if !ok {
		return []models.DiscoveryEvent{}
	}

	// Filter events from the requested sequence
	events := make([]models.DiscoveryEvent, 0)
	for _, evt := range discovery.Events {
		if evt.Seq > fromSeq {
			events = append(events, evt)
		}
	}

	return events
}

// discoveryKey creates a composite key for discovery deduplication
func discoveryKey(userKey, queryNorm string) string {
	return userKey + "|" + queryNorm
}

// generateID generates a random ID
func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
