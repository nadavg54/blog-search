package urls

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRSSParser_ParseFromURL(t *testing.T) {

	/*

			<title>Datadog | The Monitor blog</title>
		<description>Check out The Monitor, Datadog's main blog, to learn more about new Datadog products and features, integrations, and more.</description>
		<link>https://www.datadoghq.com/blog/</link>
		<lastBuildDate>Fri, 12 Dec 2025 19:06:06 GMT</lastBuildDate>
		<language>en-us</language>
		<copyright>Datadog, Inc. All rights reserved.</copyright>
		<item>
		<title>Highlights from AWS re:Invent 2025: Making sense of applied AI, trust, and going faster</title>
		<link>https://www.datadoghq.com/blog/aws-reinvent-2025-recap/</link>
		<guid isPermaLink="true">https://www.datadoghq.com/blog/aws-reinvent-2025-recap/</guid>
		<description>Learn about the top themes, presentations, and product releases from AWS re:Invent 2025.</description>
		<pubDate>Thu, 11 Dec 2025 00:00:00 GMT</pubDate>
		</item>
		<item>
		<title>This Month in Datadog - December 2025</title>
		<link>https://www.datadoghq.com/blog/this-month-in-datadog-december-2025/</link>
		<guid isPermaLink="true">https://www.datadoghq.com/blog/this-month-in-datadog-december-2025/</guid>
		<description>In December’s This Month in Datadog, get up to speed on our announcements from AWS re:Invent, like CloudPrem, Storage Management, and more.</description>
		<pubDate>Thu, 11 Dec 2025 00:00:00 GMT</pubDate>
		</item>

	*/

	// Create a mock RSS server

	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Test Feed</title>
		<link>https://example.com</link>
		<item>
			<title>Article 1</title>
			<link>https://example.com/article1</link>
		</item>
		<item>
			<title>Article 2</title>
			<link>https://example.com/article2</link>
		</item>
		<item>
			<title>Article 3</title>
			<link>https://example.com/article3</link>
		</item>
	</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssXML))
	}))
	defer server.Close()

	parser := NewRSSParser()
	urls, err := parser.Fetch(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse RSS feed: %v", err)
	}

	if len(urls) != 3 {
		t.Fatalf("Expected 3 URLs, got %d", len(urls))
	}

	// Verify URLs and titles
	expectedURLs := map[string]string{
		"https://example.com/article1": "Article 1",
		"https://example.com/article2": "Article 2",
		"https://example.com/article3": "Article 3",
	}

	for _, url := range urls {
		expectedTitle, exists := expectedURLs[url.Location]
		if !exists {
			t.Errorf("Unexpected URL: %s", url.Location)
		}
		if url.Title != expectedTitle {
			t.Errorf("Expected title '%s' for URL %s, got '%s'", expectedTitle, url.Location, url.Title)
		}
	}
}

func TestRSSParser_ParseFromURL_AtomFeed(t *testing.T) {
	// Test Atom feed format
	atomXML := `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
	<title>Test Atom Feed</title>
	<entry>
		<title>Atom Article 1</title>
		<link href="https://example.com/atom1"/>
	</entry>
	<entry>
		<title>Atom Article 2</title>
		<link href="https://example.com/atom2"/>
	</entry>
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write([]byte(atomXML))
	}))
	defer server.Close()

	parser := NewRSSParser()
	urls, err := parser.Fetch(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse Atom feed: %v", err)
	}

	if len(urls) < 2 {
		t.Fatalf("Expected at least 2 URLs, got %d", len(urls))
	}
}

func TestRSSParser_ParseFromURL_EmptyFeed(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Empty Feed</title>
		<link>https://example.com</link>
	</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssXML))
	}))
	defer server.Close()

	parser := NewRSSParser()
	_, err := parser.Fetch(server.URL)
	if err == nil {
		t.Error("Expected error for empty feed, got nil")
	}
}

func TestRSSParser_ParseFromURL_InvalidURL(t *testing.T) {
	parser := NewRSSParser()
	_, err := parser.Fetch("http://invalid-url-that-does-not-exist-12345.com/feed")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestRSSParser_ParseFromURL_DatadogFormat(t *testing.T) {
	// Test RSS format from Datadog blog
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
	<channel>
		<title>Datadog | The Monitor blog</title>
		<description>Check out The Monitor, Datadog's main blog, to learn more about new Datadog products and features, integrations, and more.</description>
		<link>https://www.datadoghq.com/blog/</link>
		<lastBuildDate>Fri, 12 Dec 2025 19:06:06 GMT</lastBuildDate>
		<language>en-us</language>
		<copyright>Datadog, Inc. All rights reserved.</copyright>
		<item>
			<title>Highlights from AWS re:Invent 2025: Making sense of applied AI, trust, and going faster</title>
			<link>https://www.datadoghq.com/blog/aws-reinvent-2025-recap/</link>
			<guid isPermaLink="true">https://www.datadoghq.com/blog/aws-reinvent-2025-recap/</guid>
			<description>Learn about the top themes, presentations, and product releases from AWS re:Invent 2025.</description>
			<pubDate>Thu, 11 Dec 2025 00:00:00 GMT</pubDate>
		</item>
		<item>
			<title>This Month in Datadog - December 2025</title>
			<link>https://www.datadoghq.com/blog/this-month-in-datadog-december-2025/</link>
			<guid isPermaLink="true">https://www.datadoghq.com/blog/this-month-in-datadog-december-2025/</guid>
			<description>In December's This Month in Datadog, get up to speed on our announcements from AWS re:Invent, like CloudPrem, Storage Management, and more.</description>
			<pubDate>Thu, 11 Dec 2025 00:00:00 GMT</pubDate>
		</item>
	</channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(rssXML))
	}))
	defer server.Close()

	parser := NewRSSParser()
	urls, err := parser.Fetch(server.URL)
	if err != nil {
		t.Fatalf("Failed to parse Datadog RSS feed: %v", err)
	}

	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(urls))
	}

	// Verify URLs and titles match the expected format
	expectedURLs := map[string]string{
		"https://www.datadoghq.com/blog/aws-reinvent-2025-recap/":             "Highlights from AWS re:Invent 2025: Making sense of applied AI, trust, and going faster",
		"https://www.datadoghq.com/blog/this-month-in-datadog-december-2025/": "This Month in Datadog - December 2025",
	}

	for _, url := range urls {
		expectedTitle, exists := expectedURLs[url.Location]
		if !exists {
			t.Errorf("Unexpected URL: %s", url.Location)
			continue
		}
		if url.Title != expectedTitle {
			t.Errorf("Expected title '%s' for URL %s, got '%s'", expectedTitle, url.Location, url.Title)
		}
	}
}
