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
	"blog-search/pkg/httpclient"
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
// Uses CloudflareClient to avoid 403 errors from Cloudflare-protected sites
func fetchHTML(url string) (string, error) {
	client := httpclient.NewClient(httpclient.CloudflareClient)

	resp, err := client.Get(url)
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
