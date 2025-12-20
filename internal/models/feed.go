package models

import "time"

type Feed struct {
	ID          int       `form:"-"`
	Title       string    `form:"title"`
	Description string    `form:"description"`
	Link        string    `form:"link"`
	ImageURL    string    `form:"image_url"`
	CreatedAt   time.Time `form:"-"`
}

type FeedService interface {
	CreateFeed(feed *Feed) error
	GetFeedByID(id int) (*Feed, error)
	GetLatestFeeds() ([]*Feed, error)
}
