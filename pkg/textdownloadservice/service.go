package textdownloadservice

import (
	"context"
	"fmt"

	"blog-search/pkg/db"
	"blog-search/pkg/filter"
	"blog-search/pkg/manager"
	"blog-search/pkg/parser"
)

// Service handles downloading and processing articles from sitemaps and RSS feeds
type Service struct {
	dbClient *db.Client
	manager  *manager.Manager
	parsers  []parser.Parser
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

	// Initialize parsers in order: file, sitemap, then RSS
	parsers := []parser.Parser{
		parser.NewFileParser(),
		parser.NewSitemapParser(),
		parser.NewRSSParser(),
	}

	return &Service{
		dbClient: config.DBClient,
		manager:  mgr,
		parsers:  parsers,
	}
}

// DownloadFromSitemap downloads articles from the given URL (tries sitemap, then RSS)
func (s *Service) DownloadFromSitemap(ctx context.Context, feedURL string, maxEntries int) error {
	var urls []string
	var err error

	// Try each parser until one succeeds
	for i, p := range s.parsers {
		parsedURLs, parseErr := p.ParseFromURL(feedURL)
		if parseErr != nil {
			// Try next parser if this one fails
			if i < len(s.parsers)-1 {
				continue
			}
			// Last parser failed, return error
			return fmt.Errorf("all parsers failed, last error: %w", parseErr)
		}

		if len(parsedURLs) == 0 {
			// Parser succeeded but no URLs found, try next parser
			if i < len(s.parsers)-1 {
				continue
			}
			return fmt.Errorf("no URLs found in feed")
		}

		// Parser succeeded, extract URLs
		urls = make([]string, 0, len(parsedURLs))
		for _, url := range parsedURLs {
			urls = append(urls, url.Location)
		}
		break
	}

	if len(urls) == 0 {
		return fmt.Errorf("failed to parse feed from any parser")
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
