package pipeline

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

// HTTPContentProcessor implements ContentProcessor by fetching HTML from URLs
// and extracting content using the content package
type HTTPContentProcessor struct {
	client    *httpclient.HTTPClient
	extractor content.Extractor
}

// NewHTTPContentProcessor creates a new HTTP content processor
func NewHTTPContentProcessor() *HTTPContentProcessor {
	return &HTTPContentProcessor{
		client:    httpclient.NewClient(httpclient.CloudflareClient),
		extractor: nil, // nil means use default behavior
	}
}

// NewHTTPContentProcessorWithClient creates a new HTTP content processor with a custom client type
func NewHTTPContentProcessorWithClient(clientType httpclient.ClientType) *HTTPContentProcessor {
	return &HTTPContentProcessor{
		client:    httpclient.NewClient(clientType),
		extractor: nil, // nil means use default behavior
	}
}

// NewHTTPContentProcessorWithExtractor creates a new HTTP content processor with a custom extractor
func NewHTTPContentProcessorWithExtractor(extractor content.Extractor) *HTTPContentProcessor {
	return &HTTPContentProcessor{
		client:    httpclient.NewClient(httpclient.CloudflareClient),
		extractor: extractor,
	}
}

// SetExtractor sets a custom extractor for the processor
func (p *HTTPContentProcessor) SetExtractor(extractor content.Extractor) {
	p.extractor = extractor
}

// ProcessContent fetches HTML from the URL, extracts text and title, and returns an Article
// If an extractor is set, it uses that; otherwise, it uses the default extraction functions
func (p *HTTPContentProcessor) ProcessContent(ctx context.Context, url string) (*domain.Article, error) {
	// Fetch HTML content
	htmlContent, err := p.fetchHTML(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HTML: %w", err)
	}

	var text, title string

	// Use custom extractor if provided, otherwise use default functions
	if p.extractor != nil {
		text, err = p.extractor.ExtractText(htmlContent)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text: %w", err)
		}

		title, err = p.extractor.ExtractTitle(htmlContent)
		if err != nil {
			return nil, fmt.Errorf("failed to extract title: %w", err)
		}
	} else {
		// Default behavior: use package-level functions
		text, err = content.ExtractText(htmlContent)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text: %w", err)
		}

		title, err = content.ExtractTitle(htmlContent)
		if err != nil {
			return nil, fmt.Errorf("failed to extract title: %w", err)
		}
	}

	// Create article document
	article := &domain.Article{
		URL:       url,
		Title:     title,
		Text:      text,
		CrawledAt: time.Now(),
	}

	return article, nil
}

// fetchHTML fetches HTML content from a URL
// Uses the configured HTTP client
func (p *HTTPContentProcessor) fetchHTML(url string) (string, error) {
	resp, err := p.client.Get(url)
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

// DBContentSaver implements ContentSaver by saving articles to a MongoDB database
type DBContentSaver struct {
	dbClient *db.Client
}

// NewDBContentSaver creates a new database content saver
func NewDBContentSaver(dbClient *db.Client) *DBContentSaver {
	return &DBContentSaver{
		dbClient: dbClient,
	}
}

// SaveArticle saves an article to the database
func (s *DBContentSaver) SaveArticle(ctx context.Context, article *domain.Article) error {
	return s.dbClient.SaveArticle(ctx, article)
}
