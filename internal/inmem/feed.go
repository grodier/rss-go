package inmem

import (
	"time"

	"github.com/grodier/rss-go/internal/models"
)

type FeedService struct {
	feeds []*models.Feed
}

func NewFeedService() *FeedService {
	fs := &FeedService{}

	// Seed with dummy feeds for development
	fs.CreateFeed(&models.Feed{
		Title:       "CSS-Tricks",
		Description: "Tips, tricks, and techniques on using Cascading Style Sheets.",
		Link:        "https://css-tricks.com/feed/",
		ImageURL:    "https://i0.wp.com/css-tricks.com/wp-content/uploads/2021/07/akqRGyta_400x400.png",
	})

	fs.CreateFeed(&models.Feed{
		Title:       "Go Blog",
		Description: "The Go Programming Language Blog",
		Link:        "https://go.dev/blog/feed.atom",
		ImageURL:    "",
	})

	return fs
}

func (s *FeedService) CreateFeed(feed *models.Feed) error {
	feed.ID = len(s.feeds) + 1
	feed.CreatedAt = time.Now()
	s.feeds = append(s.feeds, feed)

	return nil
}

func (s *FeedService) GetFeedByID(id int) (*models.Feed, error) {
	for _, feed := range s.feeds {
		if feed.ID == id {
			return feed, nil
		}
	}
	return nil, models.ErrNoRecord
}

func (s *FeedService) GetLatestFeeds() ([]*models.Feed, error) {
	if len(s.feeds) == 0 {
		return []*models.Feed{}, nil
	}

	start := len(s.feeds) - 10
	if start < 0 {
		start = 0
	}

	return s.feeds[start:], nil
}
