package models

import "time"

type Feed struct {
	ID          int       `form:"-"`
	Title       string    `form:"title"`
	Description string    `form:"description"`
	CreatedAt   time.Time `form:"-"`
}

type FeedService interface {
	CreateFeed(feed *Feed) error
	GetFeedByID(id int) (*Feed, error)
	GetLatestFeeds() ([]*Feed, error)
}
