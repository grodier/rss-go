# Decision Record: RSS Feed Search and Discovery Feature

**Date:** 2025-12-21
**Status:** Implemented (Refactored 2025-12-21)
**Decision Makers:** Development Team

## Table of Contents

- [Overview](#overview)
- [Feature Description](#feature-description)
- [Architecture](#architecture)
  - [Backend Components](#backend-components)
  - [Frontend Components](#frontend-components)
- [Endpoints & Routes](#endpoints--routes)
- [Data Flow](#data-flow)
- [Testing](#testing)
- [Future Improvements](#future-improvements)
- [Files Added/Modified](#files-addedmodified)

---

## Overview

This feature adds comprehensive RSS feed search and discovery capabilities to the application. Users can search for feeds by URL, domain, or keyword. The system first searches known feeds, then optionally triggers asynchronous discovery for URL-like queries that don't have known results.

### Architectural Refactoring (2025-12-21)

The initial implementation stored feeds separately in `DiscoveryStore.knownFeeds`. This created several issues:
- No user-scoped feed subscriptions (all feeds were global)
- Duplicate storage concepts (`Feed` vs `FeedCandidate`)
- Inconsistent deduplication (DiscoveryStore had it, FeedService didn't)

**Refactored Architecture:**
- Centralized feed storage in `FeedService` with URL-based deduplication
- User-scoped subscriptions via `userFeeds` map (many-to-many relationship)
- Single source of truth: `FeedService` is canonical, `DiscoveryStore` delegates to it
- Discovered feeds automatically persist via `FeedService.GetOrCreateFeed()`
- Shared feed records across users (one feed, multiple subscriptions)

### Key Capabilities

1. **Known Feed Search** - Instant search across pre-indexed and previously discovered feeds
2. **Autocomplete** - Real-time suggestions as users type
3. **Async Discovery** - Background discovery of new feeds from URLs/domains
4. **Real-time Updates** - Server-Sent Events (SSE) for live discovery progress
5. **Feed Deduplication** - Enforced at FeedService level via `feedsByURL` map
6. **User-Scoped Subscriptions** - Each user has independent subscription list

---

## Feature Description

### User Flow

1. **User visits `/feeds/search`** - Sees search interface with autocomplete
2. **User types query** - Autocomplete shows matching known feeds
3. **User submits search** - System searches known feeds immediately
4. **If URL-like and no results** - Async discovery job starts automatically
5. **Discovery progress** - User sees real-time updates via SSE
6. **Discovery completes** - Results appear on page, feeds added to known index
7. **Subsequent searches** - Previously discovered feeds return as "known"

### Discovery Logic

The system decides whether to trigger discovery based on:

- ✅ Query is URL-like (contains dot, has http/https prefix)
- ✅ No known results exist
- ✅ No discovery already in progress for this user + query
- ❌ Skip if user already has pending/completed discovery for same query

---

## Architecture

### Backend Components

#### 1. **FeedService** (`internal/inmem/feed.go`)

**Responsibility:** Central feed storage with user-scoped subscriptions

**Key Methods:**
- `GetOrCreateFeed(feedURL, title, description, siteURL, source, confidence)` - Create or retrieve feed by URL (deduplicates)
- `SubscribeToFeed(userID, feedURL)` - Subscribe user to feed (creates feed if needed)
- `UnsubscribeFromFeed(userID, feedID)` - Remove user subscription
- `GetUserFeeds(userID)` - Get all feeds for a user
- `IsUserSubscribed(userID, feedID)` - Check subscription status
- `SearchFeeds(query)` - Search all feeds by title/URL (case-insensitive)
- `GetFeedByID(feedID)` - Retrieve feed by ID
- `GetFeedByURL(feedURL)` - Retrieve feed by URL

**Data Structures:**
```go
type FeedService struct {
    mu         sync.RWMutex
    feeds      []*models.Feed              // All feeds (sequential IDs)
    feedsByURL map[string]*models.Feed     // Index by feed_url for uniqueness
    feedsByID  map[int]*models.Feed        // Index by ID for fast lookup
    userFeeds  map[int]map[int]bool        // userID -> set of feedIDs (subscriptions)
    nextID     int                          // Auto-increment ID
}
```

**Seeded Feeds:**
- CSS-Tricks
- Go Blog
- Hacker News
- The Verge
- TechCrunch

**Thread Safety:** All operations protected by `sync.RWMutex`

#### 2. **Discovery Service** (`internal/discovery/service.go`)

**Responsibility:** Orchestrates feed discovery operations

**Dependencies:**
- `DiscoveryStore` - Manages discovery sessions and events
- `FeedService` - Persists discovered feeds (injected dependency)
- `JobQueue` - Async job processing

**Key Methods:**
- `ShouldDiscover(userKey, queryNorm, knownResults)` - Determines if discovery should run
- `StartDiscovery(ctx, userKey, queryNorm, queryRaw)` - Initiates async discovery job
- `discoverFeeds(ctx, targetURL)` - Fetches and parses RSS/Atom feeds
- `parseRSS(body)` - Parses RSS 2.0 feeds
- `parseAtom(body)` - Parses Atom feeds

**Features:**
- HTTP client with timeout (10s) and redirect limits (5)
- SSRF protection (blocks private IPs)
- Response size limit (5MB)
- Test mode flag for localhost testing

**Discovery Job:**
- Implements `queue.Job` interface
- Deduplication by `discovery:{id}` key
- Emits progress events to SSE stream
- **Persists discovered feeds to FeedService via `GetOrCreateFeed()`**

#### 3. **Discovery Store** (`internal/inmem/discovery.go`)

**Responsibility:** In-memory storage for discovery sessions and events

**Dependencies:**
- `FeedService` - Delegates feed search and storage (injected dependency)

**Key Methods:**
- `SearchKnown(queryNorm)` - Delegates to `FeedService.SearchFeeds()`, converts to FeedCandidates
- `Suggest(partial)` - Delegates to `FeedService.SearchFeeds()`, limits to 10 results
- `CreateOrGetDiscovery(userKey, queryNorm, queryRaw)` - Get or create discovery session
- `GetDiscovery(id)` - Retrieve discovery by ID
- `GetDiscoveryByUserAndQuery(userKey, queryNorm)` - Check if discovery exists
- `GetDiscoveryEvents(id, fromSeq)` - Get SSE events from sequence number
- `AppendDiscoveryEvent(id, evt)` - Add event to discovery stream (max 100 events)
- `UpdateDiscovery(id, fn)` - Atomically update discovery state

**Data Structures:**
```go
type DiscoveryStore struct {
    mu           sync.RWMutex
    feedService  models.FeedService         // Feed service for searching/persisting feeds
    discoveries  map[string]*Discovery      // Active discoveries by ID
    discoveryIdx map[string]string          // (userKey|queryNorm) → discoveryID
}
```

**Architecture Change:** No longer stores feeds directly. All feed operations delegate to `FeedService`.

#### 4. **Job Queue** (`internal/queue/inmem.go`)

**Responsibility:** In-memory job queue with worker pool

**Features:**
- Buffered channel (100 jobs)
- Configurable worker pool
- Deduplication by job key
- Graceful shutdown

**Usage:**
```go
queue := queue.NewInMemQueue(workers, logger)
queue.Start(ctx)
defer queue.Stop(ctx)
```

#### 5. **Models**

**Feed Model** (`internal/models/feed.go`)

```go
type Feed struct {
    ID          int
    FeedURL     string    // Unique feed identifier
    Title       string
    Description string
    SiteURL     string    // Website URL
    ImageURL    string
    Source      string    // "manual" | "known" | "discovered"
    Confidence  int       // Discovery confidence score (0-100)
    CreatedAt   time.Time
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
```

**Discovery Models** (`internal/models/discovery.go`)

```go
type FeedCandidate struct {
    ID         string
    Title      string
    FeedURL    string
    SiteURL    string
    Source     string // "known" | "discovered"
    Confidence int    // 0-100
    Reason     string
}

type Discovery struct {
    ID        string
    UserKey   string
    QueryRaw  string
    QueryNorm string
    Status    string // "pending" | "resolved_found" | "resolved_none" | "error"
    Message   string
    Results   []FeedCandidate
    Events    []DiscoveryEvent
    Seq       int64
    CreatedAt time.Time
    UpdatedAt time.Time
}

type DiscoveryEvent struct {
    Seq     int64
    Type    string // "progress" | "results" | "done" | "error"
    Message string
    Results []FeedCandidate
    At      time.Time
}
```

**Utility Functions:**
- `NormalizeQuery(query)` - Lowercase, trim, remove prefixes/trailing slash
- `IsURLLike(query)` - Detects if query looks like a URL

#### 6. **HTTP Handlers** (`internal/server/server.go`)

**Handler Responsibilities:**

| Handler | Route | Method | Purpose |
|---------|-------|--------|---------|
| `handleFeedSearch` | `/feeds/search` | GET | Render search page with optional query/discovery |
| `handleFeedSearchPost` | `/feeds/search` | POST | Process search, trigger discovery if needed |
| `handleFeedSuggest` | `/feeds/suggest` | GET | Return autocomplete suggestions as JSON |
| `handleFeedDiscoveryEvents` | `/feeds/discover/{id}/events` | GET | SSE endpoint for discovery progress |
| `handleFeedSubscribe` | `/feeds/subscribe` | POST | Subscribe user to feed by URL |
| `handleFeedUnsubscribe` | `/feeds/{id}/unsubscribe` | POST | Unsubscribe user from feed |
| `handleFeedsList` | `/feeds` | GET | Show user's subscribed feeds (user-scoped) |
| `handleRootView` | `/` | GET | Homepage (user-scoped feeds if authenticated) |

**handleFeedSearchPost Flow:**
1. Validate query parameter
2. Normalize query
3. Search known feeds
4. Determine if discovery should run
5. Start discovery if needed
6. Return JSON (AJAX) or redirect (form submit)

**handleFeedDiscoveryEvents Flow:**
1. Extract discovery ID from URL
2. Parse `?lastEventId` query param
3. Set SSE headers (`text/event-stream`)
4. Stream events in loop with 1s polling
5. Client reconnects on disconnect

---

### Frontend Components

#### 1. **Search Page** (`internal/ui/html/pages/feed_search.tmpl.html`)

**Features:**
- Search form with autocomplete
- Discovery status box with spinner
- Results list with source badges
- JavaScript-enhanced with graceful degradation

**Template Data:**
```go
{
    CSRFToken  string
    Query      string
    Results    []FeedCandidate
    Discovery  *Discovery
}
```

#### 2. **Search JavaScript** (`internal/ui/static/js/feed-search.js`)

**Functionality:**

**1. Autocomplete**
- Debounced input (300ms)
- Fetch suggestions from `/feeds/suggest?q={query}`
- Keyboard navigation (up/down arrows, enter, escape)
- Click to select suggestion

**2. AJAX Search**
- Intercept form submit
- POST to `/feeds/search` with `Accept: application/json`
- Update results without page reload
- Trigger discovery monitoring if discovery ID returned

**3. SSE Discovery Monitoring**
- Connect to `/feeds/discover/{id}/events`
- Update status box in real-time
- Append new results as they arrive
- Auto-reconnect on disconnect
- Handle completion/error states

**Event Handlers:**
```javascript
{
  progress: updateDiscoveryStatus(message),
  results: appendResults(results),
  done: markComplete(message),
  error: showError(message)
}
```

#### 3. **Navigation Updates**

Added "Search" link to all authenticated page navbars:
- `dashboard.tmpl.html`
- `feeds_list.tmpl.html`
- `feed_create.tmpl.html`
- `feed.tmpl.html`

---

## Endpoints & Routes

### Public Routes (Authenticated Users)

| Endpoint | Method | Used In | Purpose |
|----------|--------|---------|---------|
| `/feeds/search` | GET | Nav links | Render search page |
| `/feeds/search` | POST | Search form, AJAX | Execute search |
| `/feeds/suggest` | GET | Autocomplete JS | Get suggestions |
| `/feeds/discover/{id}/events` | GET | SSE monitoring JS | Stream discovery events |

### Route Registration (`internal/server/router.go`)

```go
// Protected routes requiring authentication
r.Use(s.sessionManager.LoadAndSave)
r.Use(s.preventCSRF)
r.Use(s.authenticate)
r.Use(s.requireAuthentication)

r.Get("/feeds/search", s.handleFeedSearch)
r.Post("/feeds/search", s.handleFeedSearchPost)
r.Get("/feeds/suggest", s.handleFeedSuggest)
r.Get("/feeds/discover/{id}/events", s.handleFeedDiscoveryEvents)
```

### URL Parameters

**Query Parameters:**
- `?q={query}` - Search query (feed search, autocomplete)
- `?d={discoveryID}` - Discovery ID to monitor (feed search GET)
- `?lastEventId={seq}` - Last received event sequence (SSE reconnection)

**Path Parameters:**
- `{id}` - Discovery ID (SSE events endpoint)

---

## Data Flow

### Scenario 1: Search Known Feed

```
User types "hacker" → Autocomplete suggests "Hacker News"
                    ↓
User hits Enter → POST /feeds/search?q=hacker
                    ↓
Server searches known feeds → Returns Hacker News
                    ↓
Results displayed instantly (no discovery)
```

### Scenario 2: Discover New Feed

```
User searches "example.com" → POST /feeds/search?q=example.com
                            ↓
FeedService.SearchFeeds() → No results found
                            ↓
IsURLLike? Yes → ShouldDiscover? Yes
                            ↓
Create Discovery{id: "abc123", status: "pending"}
                            ↓
Enqueue DiscoveryJob → Worker picks up job
                            ↓
Fetch https://example.com → Parse RSS/Atom → Extract feeds
                            ↓
For each discovered feed:
  FeedService.GetOrCreateFeed() → Persist with source="discovered"
                            ↓
Emit SSE events: "progress" → "results" → "done"
                            ↓
Update Discovery{status: "resolved_found"}
                            ↓
Client receives SSE → Results appear in real-time
                            ↓
Next search for "example.com" → FeedService returns persisted feed (no discovery)
```

### Scenario 3: Concurrent Searches

```
User A searches "example.com" → Discovery starts (id: "abc123")
                              ↓
User B searches "example.com" → Check existing discovery
                              ↓
Discovery already pending → Return existing id: "abc123"
                              ↓
Both users watch same SSE stream → Both get results
```

---

## Testing

### Test Coverage

#### **FeedService Tests** (`internal/inmem/feed_test.go`)

**9 test functions, all passing:**

1. `TestGetOrCreateFeed` - Create new feed, deduplication by FeedURL
2. `TestSubscribeToFeed` - Subscribe to new/existing feed, multiple users, idempotency
3. `TestGetUserFeeds` - User-scoped feed lists, user isolation
4. `TestUnsubscribeFromFeed` - Subscription removal, feed persistence after unsubscribe
5. `TestIsUserSubscribed` - Subscription status checking
6. `TestSearchFeeds` - Search by title/URL, case-insensitive, empty query handling
7. `TestGetFeedByID` - Retrieve by ID, non-existent handling
8. `TestGetFeedByURL` - Retrieve by URL, non-existent handling
9. `TestConcurrentSubscriptions` - Thread safety with 100+ concurrent operations

#### **Discovery Store Tests** (`internal/inmem/discovery_test.go`)

**10 test functions (1 skipped), all passing:**

1. `TestSuggest` - Skipped (delegates to FeedService, tested in feed_test.go)
2. `TestCreateOrGetDiscovery` - Create new, get existing, per-user isolation
3. `TestGetDiscovery` - Retrieve by ID, non-existent handling
4. `TestGetDiscoveryByUserAndQuery` - User+query lookup, isolation
5. `TestAppendDiscoveryEvent` - Event streaming, trimming (max 100)
6. `TestUpdateDiscovery` - Atomic updates, non-existent handling
7. `TestGetDiscoveryEvents` - Full stream, from sequence, filtering
8. `TestNormalizeQuery` - Lowercase, trim, remove prefixes/suffixes
9. `TestIsURLLike` - URL detection patterns

#### **Discovery Service Tests** (`internal/discovery/service_test.go`)

**9 test functions, all passing:**

1. `TestShouldDiscover` - Decision logic for all scenarios
2. `TestStartDiscovery` - New creation, existing pending, completed reset
3. `TestDiscoveryJob_ParseRSS` - RSS 2.0 XML parsing
4. `TestDiscoveryJob_ParseAtom` - Atom feed XML parsing
5. `TestDiscoveryJob_Run` - End-to-end with HTTP mock server
6. `TestDiscoveryJob_NormalizeURL` - URL normalization
7. `TestIsPrivateIP` - SSRF protection validation
8. `TestDiscoveryJob_Kind` - Job type identification
9. `TestDiscoveryJob_Key` - Deduplication key generation

### Running Tests

```bash
# Run all tests
make test

# Run with coverage report
make test/cover
```

### CI/CD

**GitHub Actions** (`.github/workflows/test.yml`)
- Triggers on push to main, all PRs
- Go 1.25.x
- Runs `go vet` and `go test -race`
- Generates coverage reports
- Uploads to Codecov (optional)

---

## Future Improvements

### High Priority

#### 1. **Persistent Storage**
- [ ] Replace in-memory FeedService with database implementation (PostgreSQL/SQLite)
- [ ] Create `feeds` table with `UNIQUE(feed_url)` constraint
- [ ] Create `user_feeds` junction table for subscriptions
- [ ] Persist known feeds across restarts
- [ ] Store discovery history for analytics
- [ ] Add feed metadata caching (last updated, error counts)

**Impact:** Currently all discovered feeds and user subscriptions are lost on server restart

**Implementation Notes:** The refactored architecture makes this straightforward:
- FeedService interface remains unchanged
- Swap `inmem.FeedService` for `database.FeedService` in application initialization
- No handler or template changes required

#### 2. **Enhanced Discovery**
- [ ] HTML link tag discovery (`<link rel="alternate" type="application/rss+xml">`)
- [ ] Common RSS URL patterns (`/feed`, `/rss`, `/atom`)
- [ ] Subdomain checking (`blog.example.com`, `news.example.com`)
- [ ] Multi-feed sites (return all discovered feeds)

**Impact:** Would significantly increase discovery success rate

#### 3. **Rate Limiting**
- [ ] Per-user discovery limits (e.g., 10 per hour)
- [ ] Global discovery queue limits
- [ ] Backoff for failed discoveries

**Impact:** Prevents abuse and resource exhaustion

### Medium Priority

#### 4. **User Subscriptions Integration** (Partially Implemented)
- [x] User-scoped feed subscriptions via FeedService
- [x] Subscribe/unsubscribe handlers
- [ ] "Subscribe" button on search results UI
- [ ] Direct feed creation from discovered results in UI
- [ ] Check if user already subscribed (show badge in search results)

**Impact:** Backend complete, needs frontend UI integration to streamline user workflow

#### 5. **Search Improvements**
- [ ] Fuzzy matching for typos
- [ ] Search ranking/relevance scores
- [ ] Filter by source (known vs discovered)
- [ ] Pagination for large result sets

**Impact:** Better user experience with many feeds

#### 6. **Discovery Quality**
- [ ] Feed validation (check for recent posts)
- [ ] Confidence scoring based on feed quality
- [ ] Detect stale/abandoned feeds
- [ ] Extract feed metadata (language, category)

**Impact:** Helps users find high-quality feeds

### Low Priority

#### 7. **Advanced Features**
- [ ] Batch import from OPML
- [ ] Feed recommendations based on subscriptions
- [ ] Popular feeds trending section
- [ ] Feed categories/tagging
- [ ] Search history per user

#### 8. **Performance**
- [ ] Full-text search indexing (Elasticsearch/Meilisearch)
- [ ] Redis caching layer
- [ ] CDN for static assets
- [ ] Database query optimization

#### 9. **Observability**
- [ ] Metrics (discovery success rate, latency, queue depth)
- [ ] Distributed tracing
- [ ] Error tracking (Sentry)
- [ ] User analytics (search patterns, popular feeds)

### Technical Debt

- [ ] Extract SSE logic into reusable component
- [ ] Refactor job queue to support different backends (Redis, SQS)
- [ ] Add request/response logging middleware
- [ ] Improve error messages (user-facing vs internal)
- [ ] Add API documentation (OpenAPI/Swagger)

---

## Files Added/Modified

### New Files

#### Backend
- `internal/discovery/service.go` - Discovery orchestration service
- `internal/discovery/service_test.go` - Service test suite
- `internal/inmem/discovery.go` - In-memory discovery store
- `internal/inmem/discovery_test.go` - Store test suite
- `internal/inmem/feed.go` - In-memory feed service with user subscriptions
- `internal/inmem/feed_test.go` - FeedService test suite (added in refactoring)
- `internal/models/discovery.go` - Discovery domain models
- `internal/models/feed.go` - Feed domain models (refactored)
- `internal/queue/queue.go` - Job queue interface
- `internal/queue/inmem.go` - In-memory queue implementation

#### Frontend
- `internal/ui/html/pages/feed_search.tmpl.html` - Search page template
- `internal/ui/static/js/feed-search.js` - Search UI interactions

#### Infrastructure
- `.github/workflows/test.yml` - CI/CD workflow
- `decisions/001-rss-feed-search-and-discovery.md` - This document

### Modified Files

#### Backend
- `cmd/web/application.go` - Add DiscoveryService and DiscoveryStore to app
- `internal/server/router.go` - Add search/discovery routes
- `internal/server/server.go` - Add search/discovery handlers

#### Frontend
- `internal/ui/html/pages/dashboard.tmpl.html` - Add Search nav link
- `internal/ui/html/pages/feeds_list.tmpl.html` - Add Search nav link
- `internal/ui/html/pages/feed_create.tmpl.html` - Add Search nav link
- `internal/ui/html/pages/feed.tmpl.html` - Add Search nav link

#### Development
- `Makefile` - Add test and test/cover targets

---

## Refactoring: From knownFeeds to FeedService (2025-12-21)

### Problem Statement

The initial implementation had fundamental architectural issues:

1. **No User-Feed Relationships**: Feeds were global, not user-scoped subscriptions
2. **Duplicate Feed Concepts**: `Feed` and `FeedCandidate` represented the same thing
3. **Inconsistent Deduplication**: DiscoveryStore had manual deduplication, FeedService didn't
4. **Discovery Inefficiencies**: Multiple users discovering the same feed could create duplicates
5. **No Unique Constraint Enforcement**: FeedService allowed duplicate feed URLs

### Solution: Centralized FeedService

Refactored in-memory implementation to mimic a normalized database schema:

**Before:**
```go
// DiscoveryStore stored feeds separately
type DiscoveryStore struct {
    knownFeeds []FeedCandidate
    feedsByURL map[string]FeedCandidate
    // ...
}
```

**After:**
```go
// FeedService is the single source of truth
type FeedService struct {
    feeds      []*models.Feed
    feedsByURL map[string]*models.Feed  // Enforces uniqueness
    feedsByID  map[int]*models.Feed
    userFeeds  map[int]map[int]bool     // User subscriptions
}

// DiscoveryStore delegates to FeedService
type DiscoveryStore struct {
    feedService models.FeedService  // Injected dependency
    discoveries map[string]*Discovery
}
```

### Key Changes

1. **Feed Deduplication**: `feedsByURL` map enforces unique feed URLs in FeedService
2. **User-Scoped Subscriptions**: `userFeeds` map tracks which users subscribe to which feeds
3. **Multi-User Efficiency**: Users share same feed records, only subscriptions grow
4. **Centralized Search**: FeedService.SearchFeeds() is used by both discovery and search handlers
5. **Single Source of Truth**: DiscoveryStore.SearchKnown() delegates to FeedService
6. **Discovery Persistence**: Discovered feeds added via `FeedService.GetOrCreateFeed()`
7. **Interface-Based Design**: Easy to swap in-memory for database implementation later

### Benefits

- **Proper Deduplication**: One feed URL = one feed record, shared across users
- **User Isolation**: Each user has independent subscription list
- **Better Scalability**: Shared feed records reduce memory usage
- **Cleaner Architecture**: Clear separation between feed storage and discovery sessions
- **Database-Ready**: In-memory structure mimics SQL schema (feeds table + user_feeds junction table)

### Migration Path to Database

When migrating to a database (PostgreSQL/SQLite):
- `feeds` slice → `feeds` table with `UNIQUE(feed_url)` constraint
- `userFeeds` map → `user_feeds` junction table with `(user_id, feed_id)` composite key
- FeedService interface unchanged → swap `inmem.FeedService` for `database.FeedService`
- No handler or template changes needed

---

## Decision Rationale

### Why In-Memory Storage?

**Pros:**
- Fast development iteration
- Simple deployment (no DB dependency)
- Good for POC/MVP
- Sufficient for single-instance deployments

**Cons:**
- Data loss on restart
- No horizontal scaling
- Limited to single server

**Decision:** Start with in-memory, migrate to DB when persistence is needed.

### Why Server-Sent Events (SSE)?

**Alternatives Considered:**
- WebSockets (bidirectional, more complex)
- Long polling (inefficient)
- Periodic AJAX polling (wasteful)

**Why SSE:**
- Unidirectional (server → client) is sufficient
- Auto-reconnection built-in
- Simpler than WebSockets
- Works over HTTP (no protocol upgrade)
- Browser support excellent

### Why Async Discovery?

**Alternatives:**
- Synchronous discovery (blocks user)
- Pre-crawling all feeds (expensive)

**Why Async:**
- Better UX (no blocking)
- Allows long-running operations
- Can queue/prioritize jobs
- Enables real-time progress updates

### Why Deduplication by FeedURL?

**Alternatives:**
- Deduplicate by title (unreliable, titles change)
- Allow duplicates (confusing UX)
- Per-user deduplication (wasteful, doesn't share work)

**Why FeedURL (Refactored):**
- FeedURL is canonical identifier for RSS/Atom feeds
- Prevents redundant discoveries across all users
- Enforced at FeedService level via `feedsByURL` map
- Multiple users can discover/subscribe to same feed without duplication
- Simple implementation with strong guarantees
- Mirrors how databases enforce uniqueness (UNIQUE constraint)

---

## Risks & Mitigations

### Risk: SSRF Attacks

**Mitigation:**
- Block private IP ranges
- Limit redirects (5 max)
- Timeout requests (10s)
- Response size limit (5MB)

### Risk: Memory Exhaustion

**Mitigation:**
- Limit job queue size (100)
- Trim old events (max 100 per discovery)
- Response size limits
- Request timeouts

### Risk: Malicious Feeds

**Mitigation:**
- XML parser limits (built into Go's xml package)
- Response size limits
- Input validation

### Risk: Discovery Spam

**Mitigation:**
- Per-user deduplication
- TODO: Add rate limiting

---

## Conclusion

This feature provides a solid foundation for feed discovery with room for enhancement. The refactored architecture (2025-12-21) is modular, testable, and database-ready, with clear separation of concerns. The async nature with real-time updates provides excellent UX while maintaining system responsiveness.

**Refactoring Achievements:**
- ✅ Centralized feed storage in FeedService with URL-based deduplication
- ✅ User-scoped subscriptions (many-to-many relationship)
- ✅ Single source of truth for feeds across discovery and user operations
- ✅ Interface-based design for easy database migration
- ✅ Comprehensive test coverage (18 test functions, all passing)
- ✅ Thread-safe concurrent operations

**Next Steps:**
1. Manual testing of full user flow (search → discover → subscribe)
2. Frontend UI for subscribe/unsubscribe on search results
3. Monitor discovery success rates
4. Implement persistent storage (PostgreSQL/SQLite) when needed
5. Prioritize improvements based on usage patterns

---

**Questions or feedback?** Please open an issue or discuss in the team channel.
