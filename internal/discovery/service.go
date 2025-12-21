package discovery

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grodier/rss-go/internal/models"
	"github.com/grodier/rss-go/internal/queue"
)

const (
	maxRedirects    = 5
	requestTimeout  = 10 * time.Second
	maxResponseSize = 5 * 1024 * 1024 // 5MB
)

// Service orchestrates feed discovery operations
type Service struct {
	store             models.DiscoveryService
	queue             queue.JobQueue
	logger            *slog.Logger
	client            *http.Client
	allowPrivateIPs   bool // For testing purposes
}

// NewService creates a new discovery service
func NewService(store models.DiscoveryService, q queue.JobQueue, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		queue:  q,
		logger: logger,
		client: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// ShouldDiscover determines if discovery should be triggered for a query
func (s *Service) ShouldDiscover(userKey, queryNorm string, knownResults []models.FeedCandidate) bool {
	s.logger.Info("evaluating if discovery should be triggered",
		"user_key", userKey,
		"query_norm", queryNorm,
		"known_results_count", len(knownResults),
	)

	// Don't discover if we have known results
	if len(knownResults) > 0 {
		s.logger.Info("skipping discovery - known results exist",
			"count", len(knownResults),
		)
		return false
	}

	// Only discover for URL-like queries
	isURLLike := models.IsURLLike(queryNorm)
	if !isURLLike {
		s.logger.Info("skipping discovery - query is not URL-like",
			"query", queryNorm,
		)
		return false
	}

	// Check if discovery already exists and is in progress (WITHOUT creating one)
	existingDiscovery, exists := s.store.GetDiscoveryByUserAndQuery(userKey, queryNorm)
	if exists {
		s.logger.Info("found existing discovery",
			"discovery_id", existingDiscovery.ID,
			"status", existingDiscovery.Status,
		)

		if existingDiscovery.Status == "pending" {
			s.logger.Info("skipping discovery - already in progress",
				"discovery_id", existingDiscovery.ID,
			)
			return false // Already in progress
		}

		// Discovery exists but completed - we can retry
		s.logger.Info("existing discovery completed, will create new one",
			"discovery_id", existingDiscovery.ID,
			"previous_status", existingDiscovery.Status,
		)
	} else {
		s.logger.Info("no existing discovery found - will create new one",
			"user_key", userKey,
			"query", queryNorm,
		)
	}

	s.logger.Info("discovery should be triggered",
		"query", queryNorm,
	)
	return true
}

// StartDiscovery creates a discovery and enqueues a discovery job
func (s *Service) StartDiscovery(ctx context.Context, userKey, queryNorm, queryRaw string) (*models.Discovery, error) {
	discovery, isNew := s.store.CreateOrGetDiscovery(userKey, queryNorm, queryRaw)

	if !isNew {
		s.logger.Info("discovery already exists",
			"discovery_id", discovery.ID,
			"status", discovery.Status,
		)

		// If discovery is already pending, return it as-is
		if discovery.Status == "pending" {
			s.logger.Info("discovery already in progress, returning existing",
				"discovery_id", discovery.ID,
			)
			return discovery, nil
		}

		// Discovery exists but completed - reset it for a new run
		s.logger.Info("resetting completed discovery for new run",
			"discovery_id", discovery.ID,
			"previous_status", discovery.Status,
		)

		s.store.UpdateDiscovery(discovery.ID, func(d *models.Discovery) {
			d.Status = "pending"
			d.Message = ""
			d.Results = make([]models.FeedCandidate, 0)
			// Keep existing events for history
		})
	}

	s.logger.Info("starting discovery",
		"discovery_id", discovery.ID,
		"query", queryRaw,
		"is_new", isNew,
	)

	// Emit initial progress event
	s.emitEvent(discovery.ID, "progress", "Starting feed discovery...", nil)

	// Enqueue discovery job
	job := &DiscoveryJob{
		DiscoveryID: discovery.ID,
		Query:       queryNorm,
		service:     s,
	}

	if err := s.queue.Enqueue(ctx, job); err != nil {
		s.logger.Error("failed to enqueue discovery job",
			"discovery_id", discovery.ID,
			"error", err,
		)

		s.emitEvent(discovery.ID, "error", "Failed to start discovery", nil)
		s.store.UpdateDiscovery(discovery.ID, func(d *models.Discovery) {
			d.Status = "error"
			d.Message = "Failed to start discovery"
		})

		return discovery, err
	}

	s.logger.Info("discovery job enqueued successfully",
		"discovery_id", discovery.ID,
	)

	return discovery, nil
}

// emitEvent is a helper to emit discovery events
func (s *Service) emitEvent(discoveryID, eventType, message string, results []models.FeedCandidate) {
	discovery, ok := s.store.GetDiscovery(discoveryID)
	if !ok {
		return
	}

	evt := models.DiscoveryEvent{
		Seq:     discovery.NextSeq(),
		Type:    eventType,
		Message: message,
		Results: results,
		At:      time.Now(),
	}

	s.store.AppendDiscoveryEvent(discoveryID, evt)
}

// DiscoveryJob represents a feed discovery job
type DiscoveryJob struct {
	DiscoveryID string
	Query       string
	service     *Service
}

// Kind returns the job type
func (j *DiscoveryJob) Kind() string {
	return "discovery"
}

// Key returns the unique job key for deduplication
func (j *DiscoveryJob) Key() string {
	return "discovery:" + j.DiscoveryID
}

// Run executes the discovery job
func (j *DiscoveryJob) Run(ctx context.Context) error {
	s := j.service

	s.logger.Info("executing discovery job",
		"discovery_id", j.DiscoveryID,
		"query", j.Query,
	)

	// Normalize the URL
	feedURL := j.normalizeURL(j.Query)

	s.emitEvent(j.DiscoveryID, "progress", fmt.Sprintf("Fetching %s...", feedURL), nil)

	// Fetch the URL
	feeds, err := j.discoverFeeds(ctx, feedURL)
	if err != nil {
		s.logger.Error("discovery failed",
			"discovery_id", j.DiscoveryID,
			"error", err,
		)

		s.emitEvent(j.DiscoveryID, "error", fmt.Sprintf("Discovery failed: %v", err), nil)
		s.store.UpdateDiscovery(j.DiscoveryID, func(d *models.Discovery) {
			d.Status = "error"
			d.Message = fmt.Sprintf("Discovery failed: %v", err)
		})
		return err
	}

	if len(feeds) == 0 {
		s.emitEvent(j.DiscoveryID, "done", "No feeds found", nil)
		s.store.UpdateDiscovery(j.DiscoveryID, func(d *models.Discovery) {
			d.Status = "resolved_none"
			d.Message = "No feeds found at this URL"
		})
		return nil
	}

	// Emit results
	s.emitEvent(j.DiscoveryID, "results", fmt.Sprintf("Found %d feed(s)", len(feeds)), feeds)
	s.emitEvent(j.DiscoveryID, "done", "Discovery complete", nil)

	s.store.UpdateDiscovery(j.DiscoveryID, func(d *models.Discovery) {
		d.Status = "resolved_found"
		d.Message = fmt.Sprintf("Found %d feed(s)", len(feeds))
		d.Results = feeds
	})

	// Add discovered feeds to known feeds index
	for _, feed := range feeds {
		s.store.AddKnownFeed(feed)
		s.logger.Info("added discovered feed to known feeds",
			"feed_url", feed.FeedURL,
			"title", feed.Title,
		)
	}

	return nil
}

// normalizeURL ensures the query has a scheme
func (j *DiscoveryJob) normalizeURL(query string) string {
	query = strings.TrimSpace(query)

	if !strings.HasPrefix(query, "http://") && !strings.HasPrefix(query, "https://") {
		return "https://" + query
	}

	return query
}

// discoverFeeds attempts to discover RSS/Atom feeds from a URL
func (j *DiscoveryJob) discoverFeeds(ctx context.Context, targetURL string) ([]models.FeedCandidate, error) {
	s := j.service

	// Basic SSRF protection: ensure it's a valid URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Block private IP ranges (basic protection)
	// Allow bypass for testing
	if !s.allowPrivateIPs && isPrivateIP(parsedURL.Hostname()) {
		return nil, fmt.Errorf("private IP addresses are not allowed")
	}

	// Fetch the URL
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "RSS-Go-Bot/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Try to parse as RSS/Atom
	feeds := j.parseFeed(body, targetURL)

	return feeds, nil
}

// parseFeed attempts to parse RSS or Atom feed
func (j *DiscoveryJob) parseFeed(body []byte, sourceURL string) []models.FeedCandidate {
	candidates := make([]models.FeedCandidate, 0)

	// Try RSS 2.0
	if feed := j.parseRSS(body); feed != nil {
		candidates = append(candidates, *feed)
	}

	// Try Atom
	if feed := j.parseAtom(body); feed != nil {
		candidates = append(candidates, *feed)
	}

	// Set source URL for all candidates
	for i := range candidates {
		candidates[i].FeedURL = sourceURL
		candidates[i].Source = "discovered"
		candidates[i].Confidence = 90
	}

	return candidates
}

// RSS feed structures
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
}

// parseRSS attempts to parse RSS 2.0 feed
func (j *DiscoveryJob) parseRSS(body []byte) *models.FeedCandidate {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}

	if feed.Channel.Title == "" {
		return nil
	}

	return &models.FeedCandidate{
		Title:   feed.Channel.Title,
		SiteURL: feed.Channel.Link,
		Reason:  "RSS 2.0 feed",
	}
}

// Atom feed structures
type atomFeed struct {
	XMLName xml.Name   `xml:"feed"`
	Title   string     `xml:"title"`
	Link    []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// parseAtom attempts to parse Atom feed
func (j *DiscoveryJob) parseAtom(body []byte) *models.FeedCandidate {
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil
	}

	if feed.Title == "" {
		return nil
	}

	// Find alternate link
	siteURL := ""
	for _, link := range feed.Link {
		if link.Rel == "alternate" || link.Rel == "" {
			siteURL = link.Href
			break
		}
	}

	return &models.FeedCandidate{
		Title:   feed.Title,
		SiteURL: siteURL,
		Reason:  "Atom feed",
	}
}

// isPrivateIP checks if a hostname is a private IP (basic SSRF protection)
func isPrivateIP(hostname string) bool {
	// Basic check for localhost and private IP ranges
	privateHosts := []string{"localhost", "127.0.0.1", "0.0.0.0", "::1"}
	for _, ph := range privateHosts {
		if hostname == ph {
			return true
		}
	}

	// Check for private IP prefixes
	privatePrefixes := []string{"10.", "172.16.", "192.168.", "169.254."}
	for _, prefix := range privatePrefixes {
		if strings.HasPrefix(hostname, prefix) {
			return true
		}
	}

	return false
}
