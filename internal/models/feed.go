package models

import "time"

type Feed struct {
	ID          int
	Title       string
	Description string
	CreatedAt   time.Time
}

type FeedService interface {
	CreateFeed(feed *Feed) error
	GetFeedByID(id int) (*Feed, error)
	GetLatestFeeds() ([]*Feed, error)
}
