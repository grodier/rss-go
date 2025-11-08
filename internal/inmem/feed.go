package inmem

import (
	"time"

	"github.com/grodier/rss-go/internal/models"
)

type FeedService struct{}

func NewFeedService() *FeedService {
	return &FeedService{}
}

func (s *FeedService) CreateFeed(feed *models.Feed) error {
	feed.ID = 1
	feed.CreatedAt = time.Now()

	return nil
}

func (s *FeedService) GetFeedByID(id int) (*models.Feed, error) {
	return &models.Feed{}, nil
}

func (s *FeedService) GetLatestFeeds() ([]*models.Feed, error) {
	return nil, nil
}
