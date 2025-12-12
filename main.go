package main

import (
	"context"
	"log"
	"net/url"
	"os"
	"strings"

	"blog-search/pkg/db"
	"blog-search/pkg/manager"
	"blog-search/pkg/sitemap"
)

func main() {
	// For now, hardcode the sitemap URL
	sitemapURL := "https://engineering.fb.com/sitemap_index.xml"

	if len(os.Args) > 1 {
		sitemapURL = os.Args[1]
	}

	// Parse sitemap
	parser := sitemap.NewParser()
	entries, err := parser.ParseFromURL(sitemapURL)
	if err != nil {
		log.Fatalf("Failed to parse sitemap: %v", err)
	}

	if len(entries) == 0 {
		log.Fatal("No entries found in sitemap")
	}

	// Filter out base URLs (homepage/root URLs)
	filteredEntries := filterBaseURLs(entries)
	if len(filteredEntries) == 0 {
		log.Fatal("No valid entries found after filtering base URLs")
	}

	// Limit to first 1000 entries
	maxEntries := 1000
	if len(filteredEntries) < maxEntries {
		maxEntries = len(filteredEntries)
	}
	filteredEntries = filteredEntries[:maxEntries]

	log.Printf("Processing %d articles...", maxEntries)

	// Initialize database client
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch", "articles")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	// Extract URLs from entries
	urls := make([]string, 0, len(filteredEntries))
	for _, entry := range filteredEntries {
		urls = append(urls, entry.Location)
	}

	// Create manager with 10 workers
	mgr := manager.NewManager(10, dbClient)

	// Process all URLs
	if err := mgr.ProcessURLs(ctx, urls); err != nil {
		log.Fatalf("Failed to process URLs: %v", err)
	}

	log.Println("All done!")
}

// filterBaseURLs filters out base/root URLs that shouldn't be crawled
func filterBaseURLs(entries []sitemap.Entry) []sitemap.Entry {
	filtered := make([]sitemap.Entry, 0, len(entries))

	for _, entry := range entries {
		if !isBaseURL(entry.Location) {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}

// isBaseURL checks if a URL is a base/root URL (e.g., https://engineering.fb.com/ or https://engineering.fb.com)
func isBaseURL(urlStr string) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		// If we can't parse it, don't filter it out (let it fail later if needed)
		return false
	}

	// Check if path is empty or just "/"
	path := strings.Trim(parsed.Path, "/")
	return path == ""
}
