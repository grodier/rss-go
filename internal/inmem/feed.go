package inmem

import (
	"errors"
	"time"

	"github.com/grodier/rss-go/internal/models"
)

type FeedService struct {
	feeds []*models.Feed
}

func NewFeedService() *FeedService {
	return &FeedService{}
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
	return nil, errors.New("no matching record found")
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
