package textdownloadservice

import (
	"context"
	"fmt"

	"blog-search/pkg/db"
	"blog-search/pkg/filter"
	"blog-search/pkg/manager"
	"blog-search/pkg/sitemap"
)

// Service handles downloading and processing articles from sitemaps
type Service struct {
	dbClient      *db.Client
	manager       *manager.Manager
	sitemapParser *sitemap.Parser
}

// Config holds configuration for the service
type Config struct {
	DBClient    *db.Client
	WorkerCount int
	MaxEntries  int
}

// NewService creates a new TextDownloadService
func NewService(config Config) *Service {
	mgr := manager.NewManager(config.WorkerCount, config.DBClient)
	parser := sitemap.NewParser()

	return &Service{
		dbClient:      config.DBClient,
		manager:       mgr,
		sitemapParser: parser,
	}
}

// DownloadFromSitemap downloads articles from the given sitemap URL
func (s *Service) DownloadFromSitemap(ctx context.Context, sitemapURL string, maxEntries int) error {
	// Parse sitemap
	entries, err := s.sitemapParser.ParseFromURL(sitemapURL)
	if err != nil {
		return fmt.Errorf("failed to parse sitemap: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("no entries found in sitemap")
	}

	// Extract URLs from entries
	urls := make([]string, 0, len(entries))
	for _, entry := range entries {
		urls = append(urls, entry.Location)
	}

	// Get already-fetched URLs from database
	fetchedURLs, err := s.dbClient.GetAllURLs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get fetched URLs: %w", err)
	}

	// Apply filters
	filters := []filter.Filter{
		filter.NewBaseURLFilter(),
		filter.NewAlreadyFetchedFilter(fetchedURLs),
	}

	filteredURLs, err := filter.FilterURLs(ctx, urls, filters...)
	if err != nil {
		return fmt.Errorf("failed to filter URLs: %w", err)
	}

	if len(filteredURLs) == 0 {
		return fmt.Errorf("no valid URLs found after filtering")
	}

	// Limit to max entries
	if len(filteredURLs) < maxEntries {
		maxEntries = len(filteredURLs)
	}
	filteredURLs = filteredURLs[:maxEntries]

	// Process all URLs using manager
	if err := s.manager.ProcessURLs(ctx, filteredURLs); err != nil {
		return fmt.Errorf("failed to process URLs: %w", err)
	}

	return nil
}
