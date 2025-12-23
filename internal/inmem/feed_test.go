package inmem

import (
	"sync"
	"testing"
)

func TestGetOrCreateFeed(t *testing.T) {
	t.Run("create new feed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed, err := service.GetOrCreateFeed(
			"https://example.com/feed.xml",
			"Example Feed",
			"A test feed",
			"https://example.com",
			"manual",
			100,
		)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if feed.FeedURL != "https://example.com/feed.xml" {
			t.Errorf("expected feed URL 'https://example.com/feed.xml', got %q", feed.FeedURL)
		}

		if feed.Title != "Example Feed" {
			t.Errorf("expected title 'Example Feed', got %q", feed.Title)
		}

		if feed.Source != "manual" {
			t.Errorf("expected source 'manual', got %q", feed.Source)
		}
	})

	t.Run("get existing feed by URL", func(t *testing.T) {
		service := newEmptyFeedService()

		feed1, _ := service.GetOrCreateFeed(
			"https://example.com/feed.xml",
			"Example Feed",
			"A test feed",
			"https://example.com",
			"manual",
			100,
		)

		// Try to create same feed again
		feed2, _ := service.GetOrCreateFeed(
			"https://example.com/feed.xml",
			"Different Title", // Should be ignored
			"Different description",
			"https://different.com",
			"discovered",
			50,
		)

		if feed1.ID != feed2.ID {
			t.Errorf("expected same feed ID, got %d and %d", feed1.ID, feed2.ID)
		}

		if feed2.Title != "Example Feed" {
			t.Errorf("expected original title 'Example Feed', got %q", feed2.Title)
		}

		if feed2.Source != "manual" {
			t.Errorf("expected original source 'manual', got %q", feed2.Source)
		}
	})
}

func TestSubscribeToFeed(t *testing.T) {
	t.Run("subscribe to new feed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed, err := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if feed.FeedURL != "https://example.com/feed.xml" {
			t.Errorf("expected feed URL 'https://example.com/feed.xml', got %q", feed.FeedURL)
		}

		// Verify subscription was created
		isSubscribed, _ := service.IsUserSubscribed(1, feed.ID)
		if !isSubscribed {
			t.Error("expected user to be subscribed to feed")
		}
	})

	t.Run("subscribe to existing feed", func(t *testing.T) {
		service := newEmptyFeedService()

		// Create feed first
		feed1, _ := service.GetOrCreateFeed(
			"https://example.com/feed.xml",
			"Example Feed",
			"A test feed",
			"https://example.com",
			"manual",
			100,
		)

		// Subscribe to it
		feed2, err := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if feed1.ID != feed2.ID {
			t.Errorf("expected same feed ID, got %d and %d", feed1.ID, feed2.ID)
		}
	})

	t.Run("multiple users subscribe to same feed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed1, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")
		feed2, _ := service.SubscribeToFeed(2, "https://example.com/feed.xml")
		feed3, _ := service.SubscribeToFeed(3, "https://example.com/feed.xml")

		// All should get the same feed
		if feed1.ID != feed2.ID || feed2.ID != feed3.ID {
			t.Errorf("expected same feed ID for all users, got %d, %d, %d", feed1.ID, feed2.ID, feed3.ID)
		}

		// All users should be subscribed
		isSubscribed1, _ := service.IsUserSubscribed(1, feed1.ID)
		isSubscribed2, _ := service.IsUserSubscribed(2, feed1.ID)
		isSubscribed3, _ := service.IsUserSubscribed(3, feed1.ID)

		if !isSubscribed1 || !isSubscribed2 || !isSubscribed3 {
			t.Error("expected all users to be subscribed")
		}
	})

	t.Run("subscribe twice to same feed is idempotent", func(t *testing.T) {
		service := newEmptyFeedService()

		feed1, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")
		feed2, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		if feed1.ID != feed2.ID {
			t.Errorf("expected same feed ID, got %d and %d", feed1.ID, feed2.ID)
		}

		// Should still be subscribed once
		feeds, _ := service.GetUserFeeds(1)
		if len(feeds) != 1 {
			t.Errorf("expected 1 feed, got %d", len(feeds))
		}
	})
}

func TestGetUserFeeds(t *testing.T) {
	t.Run("get feeds for user with subscriptions", func(t *testing.T) {
		service := newEmptyFeedService()

		// Subscribe to multiple feeds
		service.SubscribeToFeed(1, "https://example.com/feed1.xml")
		service.SubscribeToFeed(1, "https://example.com/feed2.xml")
		service.SubscribeToFeed(1, "https://example.com/feed3.xml")

		feeds, err := service.GetUserFeeds(1)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(feeds) != 3 {
			t.Errorf("expected 3 feeds, got %d", len(feeds))
		}
	})

	t.Run("get feeds for user with no subscriptions", func(t *testing.T) {
		service := newEmptyFeedService()

		feeds, err := service.GetUserFeeds(1)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(feeds) != 0 {
			t.Errorf("expected 0 feeds, got %d", len(feeds))
		}
	})

	t.Run("users see only their own feeds", func(t *testing.T) {
		service := newEmptyFeedService()

		// User 1 subscribes to feed1 and feed2
		service.SubscribeToFeed(1, "https://example.com/feed1.xml")
		service.SubscribeToFeed(1, "https://example.com/feed2.xml")

		// User 2 subscribes to feed2 and feed3
		service.SubscribeToFeed(2, "https://example.com/feed2.xml")
		service.SubscribeToFeed(2, "https://example.com/feed3.xml")

		feeds1, _ := service.GetUserFeeds(1)
		feeds2, _ := service.GetUserFeeds(2)

		if len(feeds1) != 2 {
			t.Errorf("expected user 1 to have 2 feeds, got %d", len(feeds1))
		}

		if len(feeds2) != 2 {
			t.Errorf("expected user 2 to have 2 feeds, got %d", len(feeds2))
		}

		// Verify user 1 doesn't see feed3
		for _, feed := range feeds1 {
			if feed.FeedURL == "https://example.com/feed3.xml" {
				t.Error("user 1 should not see feed3")
			}
		}

		// Verify user 2 doesn't see feed1
		for _, feed := range feeds2 {
			if feed.FeedURL == "https://example.com/feed1.xml" {
				t.Error("user 2 should not see feed1")
			}
		}
	})
}

func TestUnsubscribeFromFeed(t *testing.T) {
	t.Run("unsubscribe from existing feed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		err := service.UnsubscribeFromFeed(1, feed.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify subscription was removed
		isSubscribed, _ := service.IsUserSubscribed(1, feed.ID)
		if isSubscribed {
			t.Error("expected user to not be subscribed after unsubscribe")
		}

		// Verify feed is no longer in user's list
		feeds, _ := service.GetUserFeeds(1)
		if len(feeds) != 0 {
			t.Errorf("expected 0 feeds, got %d", len(feeds))
		}
	})

	t.Run("unsubscribe from non-existent subscription", func(t *testing.T) {
		service := newEmptyFeedService()

		// Should not error
		err := service.UnsubscribeFromFeed(1, 999)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("feed persists after one user unsubscribes", func(t *testing.T) {
		service := newEmptyFeedService()

		// Both users subscribe to same feed
		feed1, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")
		service.SubscribeToFeed(2, "https://example.com/feed.xml")

		// User 1 unsubscribes
		service.UnsubscribeFromFeed(1, feed1.ID)

		// Feed should still exist
		feed, err := service.GetFeedByID(feed1.ID)
		if err != nil {
			t.Fatalf("expected feed to still exist, got error: %v", err)
		}

		if feed.ID != feed1.ID {
			t.Errorf("expected feed ID %d, got %d", feed1.ID, feed.ID)
		}

		// User 2 should still be subscribed
		isSubscribed, _ := service.IsUserSubscribed(2, feed1.ID)
		if !isSubscribed {
			t.Error("expected user 2 to still be subscribed")
		}
	})
}

func TestIsUserSubscribed(t *testing.T) {
	t.Run("user is subscribed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		isSubscribed, err := service.IsUserSubscribed(1, feed.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !isSubscribed {
			t.Error("expected user to be subscribed")
		}
	})

	t.Run("user is not subscribed", func(t *testing.T) {
		service := newEmptyFeedService()

		feed, _ := service.SubscribeToFeed(1, "https://example.com/feed.xml")

		isSubscribed, err := service.IsUserSubscribed(2, feed.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if isSubscribed {
			t.Error("expected user to not be subscribed")
		}
	})
}

func TestSearchFeeds(t *testing.T) {
	t.Run("search by title", func(t *testing.T) {
		service := newEmptyFeedService()

		service.GetOrCreateFeed("https://example.com/tech.xml", "Tech News", "", "", "known", 100)
		service.GetOrCreateFeed("https://example.com/sports.xml", "Sports Updates", "", "", "known", 100)
		service.GetOrCreateFeed("https://example.com/tech-blog.xml", "Technology Blog", "", "", "known", 100)

		results, err := service.SearchFeeds("tech")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("search by feed URL", func(t *testing.T) {
		service := newEmptyFeedService()

		service.GetOrCreateFeed("https://example.com/feed.xml", "Example Feed", "", "", "known", 100)
		service.GetOrCreateFeed("https://other.com/feed.xml", "Other Feed", "", "", "known", 100)

		results, err := service.SearchFeeds("example.com")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}

		if results[0].FeedURL != "https://example.com/feed.xml" {
			t.Errorf("expected feed URL 'https://example.com/feed.xml', got %q", results[0].FeedURL)
		}
	})

	t.Run("search is case insensitive", func(t *testing.T) {
		service := newEmptyFeedService()

		service.GetOrCreateFeed("https://example.com/feed.xml", "Tech News", "", "", "known", 100)

		results, err := service.SearchFeeds("TECH")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("empty query returns empty results", func(t *testing.T) {
		service := newEmptyFeedService()

		service.GetOrCreateFeed("https://example.com/feed.xml", "Example Feed", "", "", "known", 100)

		results, err := service.SearchFeeds("")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}

func TestGetFeedByID(t *testing.T) {
	t.Run("get existing feed", func(t *testing.T) {
		service := newEmptyFeedService()

		created, _ := service.GetOrCreateFeed("https://example.com/feed.xml", "Example Feed", "", "", "known", 100)

		retrieved, err := service.GetFeedByID(created.ID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID %d, got %d", created.ID, retrieved.ID)
		}
	})

	t.Run("get non-existent feed", func(t *testing.T) {
		service := newEmptyFeedService()

		_, err := service.GetFeedByID(999)

		if err == nil {
			t.Error("expected error for non-existent feed")
		}
	})
}

func TestGetFeedByURL(t *testing.T) {
	t.Run("get existing feed", func(t *testing.T) {
		service := newEmptyFeedService()

		created, _ := service.GetOrCreateFeed("https://example.com/feed.xml", "Example Feed", "", "", "known", 100)

		retrieved, err := service.GetFeedByURL("https://example.com/feed.xml")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID %d, got %d", created.ID, retrieved.ID)
		}
	})

	t.Run("get non-existent feed", func(t *testing.T) {
		service := newEmptyFeedService()

		_, err := service.GetFeedByURL("https://nonexistent.com/feed.xml")

		if err == nil {
			t.Error("expected error for non-existent feed")
		}
	})
}

func TestConcurrentSubscriptions(t *testing.T) {
	t.Run("concurrent subscriptions to same feed", func(t *testing.T) {
		service := newEmptyFeedService()

		var wg sync.WaitGroup
		numGoroutines := 100

		// All goroutines subscribe to the same feed
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(userID int) {
				defer wg.Done()
				service.SubscribeToFeed(userID, "https://example.com/feed.xml")
			}(i)
		}

		wg.Wait()

		// Should only have one feed created
		results, _ := service.SearchFeeds("example.com")
		if len(results) != 1 {
			t.Errorf("expected 1 feed, got %d", len(results))
		}

		// All users should be subscribed
		feed := results[0]
		for i := 0; i < numGoroutines; i++ {
			isSubscribed, _ := service.IsUserSubscribed(i, feed.ID)
			if !isSubscribed {
				t.Errorf("expected user %d to be subscribed", i)
			}
		}
	})

	t.Run("concurrent operations on different feeds", func(t *testing.T) {
		service := newEmptyFeedService()

		var wg sync.WaitGroup
		numGoroutines := 50

		// Create different feeds concurrently
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				feedURL := "https://example.com/feed" + string(rune('0'+n)) + ".xml"
				service.SubscribeToFeed(1, feedURL)
			}(i)
		}

		wg.Wait()

		// Should have all feeds created
		feeds, _ := service.GetUserFeeds(1)
		if len(feeds) != numGoroutines {
			t.Errorf("expected %d feeds, got %d", numGoroutines, len(feeds))
		}
	})
}

