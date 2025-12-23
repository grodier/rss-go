package models

import (
	"strings"
	"sync/atomic"
	"time"
)

// FeedCandidate represents a potential feed result from search or discovery
type FeedCandidate struct {
	ID         string
	Title      string
	FeedURL    string
	SiteURL    string
	Source     string // "known" | "discovered"
	Confidence int    // 0-100 confidence score
	Reason     string // Explanation of why this feed was suggested
}

// Discovery represents an async feed discovery operation
type Discovery struct {
	ID        string
	UserKey   string // Session ID or user identifier for rate limiting
	QueryRaw  string // Original query as entered by user
	QueryNorm string // Normalized query for deduplication
	Status    string // "pending" | "resolved_found" | "resolved_none" | "error"
	Message   string // Terminal message when discovery completes
	Results   []FeedCandidate
	Events    []DiscoveryEvent
	Seq       int64 // Event sequence counter (atomic)
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NextSeq atomically increments and returns the next sequence number
func (d *Discovery) NextSeq() int64 {
	return atomic.AddInt64(&d.Seq, 1)
}

// DiscoveryEvent represents a server-sent event during discovery
type DiscoveryEvent struct {
	Seq     int64              // Event sequence number for reconnection
	Type    string             // "progress" | "results" | "done" | "error"
	Message string             // Human-readable message
	Results []FeedCandidate    // Incremental results (if Type == "results")
	At      time.Time
}

// DiscoveryService defines the interface for feed discovery operations
type DiscoveryService interface {
	// SearchKnown searches in-memory known feeds by normalized query
	SearchKnown(queryNorm string) []FeedCandidate

	// Suggest returns autocomplete suggestions for partial queries
	Suggest(partial string) []FeedCandidate

	// CreateOrGetDiscovery creates a new discovery or returns existing one
	// Returns (discovery, isNew)
	CreateOrGetDiscovery(userKey, queryNorm, queryRaw string) (*Discovery, bool)

	// GetDiscovery retrieves a discovery by ID
	GetDiscovery(id string) (*Discovery, bool)

	// GetDiscoveryByUserAndQuery checks if a discovery exists for a user+query without creating it
	GetDiscoveryByUserAndQuery(userKey, queryNorm string) (*Discovery, bool)

	// AppendDiscoveryEvent adds an event to a discovery's event stream
	AppendDiscoveryEvent(id string, evt DiscoveryEvent)

	// UpdateDiscovery atomically updates a discovery
	UpdateDiscovery(id string, fn func(*Discovery))

	// GetDiscoveryEvents returns events for SSE streaming, optionally from a sequence
	GetDiscoveryEvents(id string, fromSeq int64) []DiscoveryEvent
}

// NormalizeQuery normalizes a query string for deduplication
func NormalizeQuery(query string) string {
	// Convert to lowercase
	q := strings.ToLower(strings.TrimSpace(query))

	// Remove common URL prefixes
	q = strings.TrimPrefix(q, "http://")
	q = strings.TrimPrefix(q, "https://")
	q = strings.TrimPrefix(q, "www.")

	// Remove trailing slashes
	q = strings.TrimSuffix(q, "/")

	return q
}

// IsURLLike returns true if the query looks like a URL or domain
func IsURLLike(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))

	// Check for URL schemes
	if strings.HasPrefix(q, "http://") || strings.HasPrefix(q, "https://") {
		return true
	}

	// Check for domain-like patterns (contains a dot and no spaces)
	if strings.Contains(q, ".") && !strings.Contains(q, " ") {
		return true
	}

	return false
}
