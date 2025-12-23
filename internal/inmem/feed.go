package inmem

import (
	"strings"
	"sync"
	"time"

	"github.com/grodier/rss-go/internal/models"
)

type FeedService struct {
	mu         sync.RWMutex
	feeds      []*models.Feed
	feedsByURL map[string]*models.Feed  // Index by feed_url for uniqueness
	feedsByID  map[int]*models.Feed     // Index by ID for fast lookup
	userFeeds  map[int]map[int]bool     // userID -> set of feedIDs (subscriptions)
	nextID     int
}

func NewFeedService() *FeedService {
	fs := newEmptyFeedService()
	// Seed with known feeds (no user subscriptions by default)
	fs.seedKnownFeeds()
	return fs
}

// newEmptyFeedService creates a new feed service without seeding known feeds (for tests)
func newEmptyFeedService() *FeedService {
	return &FeedService{
		feeds:      make([]*models.Feed, 0),
		feedsByURL: make(map[string]*models.Feed),
		feedsByID:  make(map[int]*models.Feed),
		userFeeds:  make(map[int]map[int]bool),
		nextID:     1,
	}
}

func (s *FeedService) seedKnownFeeds() {
	knownFeeds := []struct {
		feedURL     string
		title       string
		description string
		siteURL     string
		imageURL    string
	}{
		{
			feedURL:     "https://css-tricks.com/feed/",
			title:       "CSS-Tricks",
			description: "Tips, tricks, and techniques on using Cascading Style Sheets.",
			siteURL:     "https://css-tricks.com",
			imageURL:    "https://i0.wp.com/css-tricks.com/wp-content/uploads/2021/07/akqRGyta_400x400.png",
		},
		{
			feedURL:     "https://go.dev/blog/feed.atom",
			title:       "Go Blog",
			description: "The Go Programming Language Blog",
			siteURL:     "https://go.dev",
			imageURL:    "",
		},
		{
			feedURL:     "https://news.ycombinator.com/rss",
			title:       "Hacker News",
			description: "Tech news aggregator",
			siteURL:     "https://news.ycombinator.com",
			imageURL:    "",
		},
		{
			feedURL:     "https://www.theverge.com/rss/index.xml",
			title:       "The Verge",
			description: "Technology news and media network",
			siteURL:     "https://www.theverge.com",
			imageURL:    "",
		},
		{
			feedURL:     "https://techcrunch.com/feed/",
			title:       "TechCrunch",
			description: "Startup and technology news",
			siteURL:     "https://techcrunch.com",
			imageURL:    "",
		},
	}

	for _, f := range knownFeeds {
		s.GetOrCreateFeed(f.feedURL, f.title, f.description, f.siteURL, "known", 100)
	}
}

// GetUserFeeds returns all feeds that a user is subscribed to
func (s *FeedService) GetUserFeeds(userID int) ([]*models.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userFeedSet, exists := s.userFeeds[userID]
	if !exists || len(userFeedSet) == 0 {
		return []*models.Feed{}, nil
	}

	result := make([]*models.Feed, 0, len(userFeedSet))
	for feedID := range userFeedSet {
		if feed, ok := s.feedsByID[feedID]; ok {
			result = append(result, feed)
		}
	}

	return result, nil
}

// SubscribeToFeed subscribes a user to a feed (creates feed if it doesn't exist)
func (s *FeedService) SubscribeToFeed(userID int, feedURL string) (*models.Feed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or create the feed
	feed, exists := s.feedsByURL[feedURL]
	if !exists {
		// Create new feed with minimal info (user will need to provide details)
		feed = &models.Feed{
			ID:         s.nextID,
			FeedURL:    feedURL,
			Title:      feedURL, // Default to URL if no title provided
			Source:     "manual",
			Confidence: 100,
			CreatedAt:  time.Now(),
		}
		s.nextID++

		s.feeds = append(s.feeds, feed)
		s.feedsByURL[feedURL] = feed
		s.feedsByID[feed.ID] = feed
	}

	// Add subscription
	if s.userFeeds[userID] == nil {
		s.userFeeds[userID] = make(map[int]bool)
	}
	s.userFeeds[userID][feed.ID] = true

	return feed, nil
}

// UnsubscribeFromFeed removes a user's subscription to a feed
func (s *FeedService) UnsubscribeFromFeed(userID, feedID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userFeeds[userID] != nil {
		delete(s.userFeeds[userID], feedID)
	}

	return nil
}

// IsUserSubscribed checks if a user is subscribed to a feed
func (s *FeedService) IsUserSubscribed(userID, feedID int) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.userFeeds[userID] == nil {
		return false, nil
	}

	return s.userFeeds[userID][feedID], nil
}

// GetFeedByID returns a feed by its ID
func (s *FeedService) GetFeedByID(feedID int) (*models.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	feed, ok := s.feedsByID[feedID]
	if !ok {
		return nil, models.ErrNoRecord
	}

	return feed, nil
}

// GetFeedByURL returns a feed by its URL
func (s *FeedService) GetFeedByURL(feedURL string) (*models.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	feed, ok := s.feedsByURL[feedURL]
	if !ok {
		return nil, models.ErrNoRecord
	}

	return feed, nil
}

// GetOrCreateFeed gets an existing feed by URL or creates it if it doesn't exist
func (s *FeedService) GetOrCreateFeed(feedURL, title, description, siteURL, source string, confidence int) (*models.Feed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if feed already exists
	if feed, exists := s.feedsByURL[feedURL]; exists {
		return feed, nil
	}

	// Create new feed
	feed := &models.Feed{
		ID:          s.nextID,
		FeedURL:     feedURL,
		Title:       title,
		Description: description,
		SiteURL:     siteURL,
		Source:      source,
		Confidence:  confidence,
		CreatedAt:   time.Now(),
	}
	s.nextID++

	s.feeds = append(s.feeds, feed)
	s.feedsByURL[feedURL] = feed
	s.feedsByID[feed.ID] = feed

	return feed, nil
}

// SearchFeeds performs a simple substring search across all feeds
func (s *FeedService) SearchFeeds(query string) ([]*models.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query == "" {
		return []*models.Feed{}, nil
	}

	queryLower := strings.ToLower(query)
	results := make([]*models.Feed, 0)

	for _, feed := range s.feeds {
		// Search in title, feedURL, siteURL, and description
		titleMatch := strings.Contains(strings.ToLower(feed.Title), queryLower)
		feedURLMatch := strings.Contains(strings.ToLower(feed.FeedURL), queryLower)
		siteURLMatch := strings.Contains(strings.ToLower(feed.SiteURL), queryLower)
		descMatch := strings.Contains(strings.ToLower(feed.Description), queryLower)

		if titleMatch || feedURLMatch || siteURLMatch || descMatch {
			results = append(results, feed)
		}
	}

	return results, nil
}
