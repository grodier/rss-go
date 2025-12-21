# Decision Record: RSS Feed Search and Discovery Feature

**Date:** 2025-12-21
**Status:** Implemented
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

### Key Capabilities

1. **Known Feed Search** - Instant search across pre-indexed and previously discovered feeds
2. **Autocomplete** - Real-time suggestions as users type
3. **Async Discovery** - Background discovery of new feeds from URLs/domains
4. **Real-time Updates** - Server-Sent Events (SSE) for live discovery progress
5. **Deduplication** - Discovered feeds are cached to avoid redundant discovery

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

#### 1. **Discovery Service** (`internal/discovery/service.go`)

**Responsibility:** Orchestrates feed discovery operations

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
- Adds successful discoveries to known feeds index

#### 2. **Discovery Store** (`internal/inmem/discovery.go`)

**Responsibility:** In-memory storage for discoveries and known feeds

**Key Methods:**
- `SearchKnown(queryNorm)` - Full-text search across title, feedURL, siteURL
- `Suggest(partial)` - Autocomplete suggestions (min 2 chars, max 10 results)
- `CreateOrGetDiscovery(userKey, queryNorm, queryRaw)` - Get or create discovery
- `AddKnownFeed(feed)` - Add feed to index (deduplicates by FeedURL)
- `GetDiscoveryEvents(id, fromSeq)` - Get SSE events from sequence number
- `AppendDiscoveryEvent(id, evt)` - Add event to discovery stream (max 100 events)

**Data Structures:**
- `knownFeeds []FeedCandidate` - Searchable feed list
- `feedsByURL map[string]FeedCandidate` - Deduplication index
- `discoveries map[string]*Discovery` - Active discoveries by ID
- `discoveryIdx map[string]string` - (userKey|queryNorm) → discoveryID

**Seeded Feeds:**
- Hacker News
- The Verge
- TechCrunch
- Ars Technica
- Wired

#### 3. **Job Queue** (`internal/queue/inmem.go`)

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

#### 4. **Models** (`internal/models/discovery.go`)

**Core Types:**

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

#### 5. **HTTP Handlers** (`internal/server/server.go`)

**Handler Responsibilities:**

| Handler | Route | Method | Purpose |
|---------|-------|--------|---------|
| `handleFeedSearch` | `/feeds/search` | GET | Render search page with optional query/discovery |
| `handleFeedSearchPost` | `/feeds/search` | POST | Process search, trigger discovery if needed |
| `handleFeedSuggest` | `/feeds/suggest` | GET | Return autocomplete suggestions as JSON |
| `handleFeedDiscoveryEvents` | `/feeds/discover/{id}/events` | GET | SSE endpoint for discovery progress |

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
No known results found → IsURLLike? Yes → ShouldDiscover? Yes
                            ↓
Create Discovery{id: "abc123", status: "pending"}
                            ↓
Enqueue DiscoveryJob → Worker picks up job
                            ↓
Fetch https://example.com → Parse RSS/Atom → Extract feeds
                            ↓
Emit SSE events: "progress" → "results" → "done"
                            ↓
Add to known feeds index → Discovery{status: "resolved_found"}
                            ↓
Client receives SSE → Results appear in real-time
                            ↓
Next search for "example.com" → Returns as "known" (no discovery)
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

#### **Discovery Store Tests** (`internal/inmem/discovery_test.go`)

**11 test functions, all passing:**

1. `TestSearchKnown` - Search by title/feedURL/siteURL, case-insensitive, empty query
2. `TestAddKnownFeed` - Add new feed, deduplication by FeedURL
3. `TestSuggest` - Autocomplete prefix matching, min length, max results
4. `TestCreateOrGetDiscovery` - Create new, get existing, per-user isolation
5. `TestGetDiscovery` - Retrieve by ID, non-existent handling
6. `TestGetDiscoveryByUserAndQuery` - User+query lookup, isolation
7. `TestAppendDiscoveryEvent` - Event streaming, trimming (max 100)
8. `TestUpdateDiscovery` - Atomic updates, non-existent handling
9. `TestGetDiscoveryEvents` - Full stream, from sequence, filtering
10. `TestNormalizeQuery` - Lowercase, trim, remove prefixes/suffixes
11. `TestIsURLLike` - URL detection patterns

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
- [ ] Replace in-memory store with database (PostgreSQL/SQLite)
- [ ] Persist known feeds across restarts
- [ ] Store discovery history for analytics
- [ ] Add feed metadata caching (last updated, error counts)

**Impact:** Currently all discovered feeds are lost on server restart

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

#### 4. **User Subscriptions Integration**
- [ ] "Subscribe" button on search results
- [ ] Direct feed creation from discovered results
- [ ] Check if user already subscribed (show badge)

**Impact:** Streamlines user workflow

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
- `internal/models/discovery.go` - Discovery domain models
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

**Why FeedURL:**
- FeedURL is canonical identifier
- Prevents redundant discoveries
- Simple implementation
- Works well in practice

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

This feature provides a solid foundation for feed discovery with room for enhancement. The architecture is modular and testable, with clear separation of concerns. The async nature with real-time updates provides excellent UX while maintaining system responsiveness.

**Next Steps:**
1. User testing and feedback
2. Monitor discovery success rates
3. Prioritize improvements based on usage patterns
4. Consider persistence layer when needed

---

**Questions or feedback?** Please open an issue or discuss in the team channel.
