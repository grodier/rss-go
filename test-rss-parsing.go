package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RSS feed structures
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
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

func main() {
	testURLs := []string{
		"https://pullrequestplaybook.com/rss.xml",
		"https://brittanyellich.com/index.xml",
		"https://news.ycombinator.com/rss",
	}

	for _, url := range testURLs {
		fmt.Printf("\n=== Testing: %s ===\n", url)
		testParseFeed(url)
	}
}

func testParseFeed(targetURL string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", targetURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		return
	}

	req.Header.Set("User-Agent", "RSS-Go-Bot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to fetch URL: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ HTTP %d: %s\n", resp.StatusCode, resp.Status)
		return
	}

	fmt.Printf("✓ HTTP 200 OK\n")
	fmt.Printf("  Content-Type: %s\n", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		fmt.Printf("❌ Failed to read response: %v\n", err)
		return
	}

	fmt.Printf("  Body size: %d bytes\n", len(body))

	// Try RSS
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && rss.Channel.Title != "" {
		fmt.Printf("✓ Parsed as RSS 2.0\n")
		fmt.Printf("  Title: %s\n", rss.Channel.Title)
		fmt.Printf("  Link: %s\n", rss.Channel.Link)
		fmt.Printf("  Description: %s\n", rss.Channel.Description)
		return
	}

	// Try Atom
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && atom.Title != "" {
		fmt.Printf("✓ Parsed as Atom\n")
		fmt.Printf("  Title: %s\n", atom.Title)
		for _, link := range atom.Link {
			if link.Rel == "alternate" || link.Rel == "" {
				fmt.Printf("  Link: %s\n", link.Href)
				break
			}
		}
		return
	}

	fmt.Printf("❌ Failed to parse as RSS or Atom\n")
	fmt.Printf("  First 200 bytes: %s\n", string(body[:min(200, len(body))]))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
