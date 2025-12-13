package parser

import (
	"fmt"

	"github.com/mmcdole/gofeed"
)

// RSSParser handles RSS/Atom feed parsing operations
type RSSParser struct {
	feedParser *gofeed.Parser
}

// NewRSSParser creates a new RSS parser
func NewRSSParser() *RSSParser {
	return &RSSParser{
		feedParser: gofeed.NewParser(),
	}
}

// ParseFromURL fetches and parses an RSS/Atom feed from the given URL
func (p *RSSParser) ParseFromURL(feedURL string) ([]URL, error) {
	feed, err := p.feedParser.ParseURL(feedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	if feed == nil || len(feed.Items) == 0 {
		return nil, fmt.Errorf("feed contains no items")
	}

	urls := make([]URL, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item.Link != "" {
			url := URL{
				Location: item.Link,
				Title:    item.Title,
			}
			urls = append(urls, url)
		}
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no valid URLs found in feed items")
	}

	return urls, nil
}
