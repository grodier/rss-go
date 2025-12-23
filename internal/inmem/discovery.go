package inmem

import (
	"crypto/rand"
	"encoding/hex"
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
	feedService  models.FeedService              // Feed service for searching feeds
	discoveries  map[string]*models.Discovery     // Active discoveries by ID
	discoveryIdx map[string]string                // (userKey, queryNorm) -> discoveryID
}

// NewDiscoveryStore creates a new in-memory discovery store
func NewDiscoveryStore(feedService models.FeedService) *DiscoveryStore {
	return &DiscoveryStore{
		feedService:  feedService,
		discoveries:  make(map[string]*models.Discovery),
		discoveryIdx: make(map[string]string),
	}
}

// SearchKnown searches feeds by normalized query using FeedService
func (s *DiscoveryStore) SearchKnown(queryNorm string) []models.FeedCandidate {
	// Delegate to FeedService
	feeds, err := s.feedService.SearchFeeds(queryNorm)
	if err != nil {
		return []models.FeedCandidate{}
	}

	// Convert Feed to FeedCandidate
	results := make([]models.FeedCandidate, 0, len(feeds))
	for _, feed := range feeds {
		results = append(results, feedToCandidate(feed))
	}

	return results
}

// Suggest returns autocomplete suggestions for partial queries
func (s *DiscoveryStore) Suggest(partial string) []models.FeedCandidate {
	if len(partial) < 2 {
		return []models.FeedCandidate{}
	}

	// Delegate to FeedService
	feeds, err := s.feedService.SearchFeeds(partial)
	if err != nil {
		return []models.FeedCandidate{}
	}

	// Convert Feed to FeedCandidate and limit to 10 results
	results := make([]models.FeedCandidate, 0, 10)
	for _, feed := range feeds {
		results = append(results, feedToCandidate(feed))
		if len(results) >= 10 {
			break
		}
	}

	return results
}

// feedToCandidate converts a Feed to a FeedCandidate
func feedToCandidate(feed *models.Feed) models.FeedCandidate {
	return models.FeedCandidate{
		ID:         generateID(), // Generate unique ID for this candidate
		Title:      feed.Title,
		FeedURL:    feed.FeedURL,
		SiteURL:    feed.SiteURL,
		Source:     feed.Source,
		Confidence: feed.Confidence,
		Reason:     "", // Can be populated based on source if needed
	}
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
