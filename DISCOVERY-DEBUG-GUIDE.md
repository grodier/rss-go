# Discovery Feature Debug Guide

## RSS Feed Parsing Test Results ✅

The RSS parsing is working correctly for all test feeds:

| Feed URL | Status | Type | Title |
|----------|--------|------|-------|
| https://pullrequestplaybook.com/rss.xml | ✅ Works | RSS 2.0 | Astro Learner \| Blog |
| https://brittanyellich.com/index.xml | ✅ Works | RSS 2.0 | Brittany Ellich's Blog |
| https://news.ycombinator.com/rss | ✅ Works | RSS 2.0 | Hacker News |

## Expected Log Flow for Discovery

### 1. When you search for a URL (e.g., "https://pullrequestplaybook.com/rss.xml")

#### Server Logs (Terminal):
```
level=INFO msg="received search POST request" method=POST
level=INFO msg="processing search query" query=https://pullrequestplaybook.com/rss.xml
level=INFO msg="normalized search query" original=https://pullrequestplaybook.com/rss.xml normalized=pullrequestplaybook.com/rss.xml
level=INFO msg="searched known feeds" query_norm=pullrequestplaybook.com/rss.xml results_count=0
level=INFO msg="got user key for rate limiting" user_key=<session-token>
level=INFO msg="evaluating if discovery should be triggered"
level=INFO msg="discovery should be triggered" query=pullrequestplaybook.com/rss.xml
level=INFO msg="starting discovery" user_key=<session-token>
level=INFO msg="starting new discovery" discovery_id=<uuid> query=https://pullrequestplaybook.com/rss.xml
level=INFO msg="attempting to enqueue job" kind=discovery key=discovery:<uuid>
level=INFO msg="job enqueued successfully" kind=discovery key=discovery:<uuid>
level=INFO msg="discovery started successfully" discovery_id=<uuid> status=pending
level=INFO msg="redirecting to search page" redirect_url=/feeds/search?q=https://pullrequestplaybook.com/rss.xml&d=<uuid>

# Worker picks up the job:
level=INFO msg="worker processing job" worker_id=0 kind=discovery key=discovery:<uuid>
level=INFO msg="executing discovery job" discovery_id=<uuid> query=pullrequestplaybook.com/rss.xml
level=INFO msg="discovery job completed" discovery_id=<uuid> feeds_found=1
```

#### Browser Console:
```javascript
[Search] Form submit event triggered
[Search] Form data: {query: "https://pullrequestplaybook.com/rss.xml", csrfToken: "present"}
[Search] Sending POST request to /feeds/search
[Search] Response received {status: 200, ok: true, contentType: "application/json"}
[Search] Response data: {results: [], discovery_id: "<uuid>"}
[Search] Displaying results {count: 0}
[Search] No results, showing empty state
[Search] Starting discovery SSE {discoveryId: "<uuid>"}
[Search] showDiscoveryStatus called {status: "pending", message: "Discovering feeds..."}
[SSE] Connecting to SSE {discoveryId: "<uuid>"}
[SSE] Creating EventSource {url: "/feeds/discover/<uuid>/events"}
[SSE] Event listeners attached
[SSE] Connection opened successfully
[SSE] Progress event received {data: "...", lastEventId: "1"}
[SSE] Parsed progress data {message: "Fetching https://pullrequestplaybook.com/rss.xml..."}
[SSE] Results event received {data: "...", lastEventId: "2"}
[SSE] Parsed results data {message: "Found 1 feed(s)", results: [{...}]}
[SSE] Appending results {count: 1}
[Search] appendResults called {count: 1}
[Search] Appending discovered result 1: {Title: "Astro Learner | Blog", ...}
[SSE] Done event received {data: "...", lastEventId: "3"}
[SSE] Parsed done data {message: "Discovery complete"}
[SSE] Closing connection - discovery complete
```

### 2. When SSE Connection is Established

#### Server Logs:
```
level=INFO msg="SSE connection request" discovery_id=<uuid> remote_addr=127.0.0.1:xxxxx
level=INFO msg="SSE connection established" discovery_id=<uuid> from_seq=0
level=INFO msg="sending initial SSE events" discovery_id=<uuid> event_count=1
level=INFO msg="sending SSE event" discovery_id=<uuid> event_type=progress event_seq=1 message="Starting feed discovery..."
level=INFO msg="starting SSE polling loop" discovery_id=<uuid>

# As events are emitted:
level=INFO msg="sending new SSE events" discovery_id=<uuid> event_count=2 discovery_status=pending
level=INFO msg="sending SSE event" discovery_id=<uuid> event_type=progress event_seq=2
level=INFO msg="sending SSE event" discovery_id=<uuid> event_type=results event_seq=3
level=INFO msg="discovery complete, closing SSE" discovery_id=<uuid> status=resolved_found
```

## Common Issues & Solutions

### Issue 1: No Discovery Triggered

**Symptoms:**
- Server logs show: `msg="skipping discovery - known results exist"`
- Or: `msg="skipping discovery - query is not URL-like"`

**Solution:**
- Known results: The feed is already in the seeded data
- Not URL-like: Query doesn't contain "." or start with "http://" or "https://"

### Issue 2: SSE Connection Not Opening

**Symptoms:**
- Browser console shows: `[SSE] Creating EventSource` but no `[SSE] Connection opened successfully`
- Server logs show no "SSE connection request"

**Possible Causes:**
1. Route not registered correctly
2. Authentication blocking SSE endpoint
3. Browser blocking EventSource

**Debug:**
- Check Network tab in DevTools for `/feeds/discover/<uuid>/events` request
- Check if request returns 401/403/404

### Issue 3: Discovery Job Not Running

**Symptoms:**
- Server logs show job enqueued but never "worker processing job"
- Job queue appears stuck

**Debug:**
- Check: `msg="job already in-flight"` - duplicate job
- Check queue status in logs for "total_inflight" count
- Verify workers started: `msg="worker started" worker_id=0`

### Issue 4: Results Not Appearing in UI

**Symptoms:**
- Browser console shows SSE events received
- But `appendResults` not called or results not showing

**Debug:**
- Check if `[Search] appendResults called` appears in console
- Check if `resultsListExists: true`
- Inspect DOM to see if `<ul id="results-list">` exists
- Check if results list has `display: none` style

## Testing Checklist

### 1. Start Server
```bash
./bin/server -port 4000
```

### 2. Open Browser DevTools
- Press F12
- Go to Console tab
- Keep Network tab open too

### 3. Navigate to Search Page
```
http://localhost:4000/feeds/search
```

You should see:
```
[Search] Initializing feed search
[Search] DOM elements: {searchForm: true, searchInput: true, ...}
[Search] Event listeners attached
[Search] Initialization complete
```

### 4. Search for Known Feed
Query: `techcrunch`

Expected:
- Immediate results (no discovery)
- Server logs: `msg="searched known feeds" results_count=1`
- Browser: `[Search] Displaying results {count: 1}`

### 5. Search for New Feed URL
Query: `https://pullrequestplaybook.com/rss.xml`

Expected:
- Discovery triggered
- SSE connection opened
- Results appear within 2-10 seconds
- Server logs show complete flow (see section 1 above)
- Browser console shows SSE events (see section 1 above)

### 6. Search for Non-URL
Query: `some random text`

Expected:
- No discovery triggered
- Server logs: `msg="skipping discovery - query is not URL-like"`
- Browser: Empty results

## Key Points

1. **RSS Parsing Works**: All test feeds parse correctly
2. **Discovery Logic**: Only triggers for URL-like queries with no known results
3. **SSE Events**: Should see progress → results → done
4. **UI Updates**: Results appended via SSE as they're discovered

## Next Steps If Still Not Working

1. **Clear Browser Cache**: Ctrl+Shift+Delete → Clear cached files
2. **Hard Reload**: Ctrl+Shift+R (Cmd+Shift+R on Mac)
3. **Check browser console**: Any JavaScript errors?
4. **Check server logs**: Compare with expected flow above
5. **Test SSE directly**: Open `http://localhost:4000/feeds/discover/<uuid>/events` in browser
   - Should see event stream (if discovery ID exists)
   - Should show keepalive comments every second

## Debug Commands

### Check if discovery was created:
Look for this in server logs:
```
level=INFO msg="starting new discovery" discovery_id=<uuid>
```

### Check if job was enqueued:
```
level=INFO msg="job enqueued successfully" kind=discovery key=discovery:<uuid>
```

### Check if worker picked up job:
```
level=INFO msg="worker processing job" worker_id=0 kind=discovery
```

### Check SSE connection:
Browser console should show:
```
[SSE] Connection opened successfully
```

Server logs should show:
```
level=INFO msg="SSE connection established" discovery_id=<uuid>
```

With this comprehensive logging, you should be able to pinpoint exactly where the issue is occurring!
