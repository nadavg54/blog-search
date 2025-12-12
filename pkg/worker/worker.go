package worker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"blog-search/pkg/db"
	"blog-search/pkg/domain"
	"blog-search/pkg/extractor"
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
	text, err := extractor.ExtractText(htmlContent)
	if err != nil {
		return fmt.Errorf("failed to extract text: %w", err)
	}

	title, err := extractor.ExtractTitle(htmlContent)
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
