package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"blog-search/pkg/db"
	"blog-search/pkg/domain"
	"blog-search/pkg/extractor"
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

	// Initialize database client
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch", "articles")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	// Take the first entry
	entry := filteredEntries[0]
	fmt.Printf("Processing entry: %s\n", entry.Location)

	// Fetch HTML content
	htmlContent, err := fetchHTML(entry.Location)
	if err != nil {
		log.Fatalf("Failed to fetch HTML: %v", err)
	}

	// Extract text and title
	text, err := extractor.ExtractText(htmlContent)
	if err != nil {
		log.Fatalf("Failed to extract text: %v", err)
	}

	title, err := extractor.ExtractTitle(htmlContent)
	if err != nil {
		log.Fatalf("Failed to extract title: %v", err)
	}

	// Create article document
	article := &domain.Article{
		URL:       entry.Location,
		Title:     title,
		Text:      text,
		CrawledAt: time.Now(),
	}

	// Save to database
	if err := dbClient.SaveArticle(ctx, article); err != nil {
		log.Fatalf("Failed to save article: %v", err)
	}

	fmt.Printf("Successfully saved article: %s\n", title)
}

// fetchHTML fetches HTML content from a URL
func fetchHTML(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
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
