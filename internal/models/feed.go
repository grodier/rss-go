package models

import "time"

type Feed struct {
	ID          int       `form:"-"`
	FeedURL     string    `form:"feed_url"`  // Unique feed identifier
	Title       string    `form:"title"`
	Description string    `form:"description"`
	SiteURL     string    `form:"site_url"`  // Renamed from Link for clarity
	ImageURL    string    `form:"image_url"`
	Source      string    `form:"-"`         // 'manual' | 'known' | 'discovered'
	Confidence  int       `form:"-"`         // Discovery confidence score (0-100)
	CreatedAt   time.Time `form:"-"`
}

type FeedService interface {
	// User-scoped subscription operations
	GetUserFeeds(userID int) ([]*Feed, error)
	SubscribeToFeed(userID int, feedURL string) (*Feed, error)
	UnsubscribeFromFeed(userID, feedID int) error
	IsUserSubscribed(userID, feedID int) (bool, error)

	// Global feed operations (for discovery and search)
	GetFeedByID(feedID int) (*Feed, error)
	GetFeedByURL(feedURL string) (*Feed, error)
	GetOrCreateFeed(feedURL, title, description, siteURL, source string, confidence int) (*Feed, error)
	SearchFeeds(query string) ([]*Feed, error)
}
