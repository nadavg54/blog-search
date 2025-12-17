package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"blog-search/pkg/content"
	"blog-search/pkg/db"
	"blog-search/pkg/domain"
)

// Worker processes articles from URLs
type Worker struct {
	dbClient *db.Client
}

// NewWorker creates a new worker
func NewWorker(dbClient *db.Client) *Worker {
	return &Worker{
		dbClient: dbClient,
	}
}

// ProcessURL processes a single URL: fetches, extracts, and saves to DB
func (w *Worker) ProcessURL(ctx context.Context, url string) error {
	// Fetch HTML content
	htmlContent, err := fetchHTML(url)
	if err != nil {
		return fmt.Errorf("failed to fetch HTML: %w", err)
	}

	// Extract text and title
	text, err := content.ExtractText(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to extract text: %w", err)
	}

	title, err := content.ExtractTitle(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to extract title: %w", err)
	}

	// Create article document
	article := &domain.Article{
		URL:       url,
		Title:     title,
		Text:      text,
		CrawledAt: time.Now(),
	}

	// Save to database
	if err := w.dbClient.SaveArticle(ctx, article); err != nil {
		return fmt.Errorf("failed to save article: %w", err)
	}

	return nil
}

// fetchHTML fetches HTML content from a URL
func fetchHTML(url string) (string, error) {
	// http.Client follows redirects by default, so we don't need to do anything special
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers to mimic a real browser and avoid 406 errors
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Don't set Accept-Encoding - let Go handle compression automatically
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := client.Do(req)
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

	bodyStr := string(body)

	// Check if we got an error page instead of actual HTML
	if strings.Contains(bodyStr, "Not Acceptable") || strings.TrimSpace(bodyStr) == "" {
		return "", fmt.Errorf("server returned error or empty response (status: %d)", resp.StatusCode)
	}

	return bodyStr, nil
}
