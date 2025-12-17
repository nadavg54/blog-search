package textdownloadservice

import (
	"context"
	"fmt"

	"blog-search/pkg/db"
	"blog-search/pkg/urls"
	"blog-search/pkg/worker"
)

// Service handles downloading and processing articles from sitemaps and RSS feeds
type Service struct {
	dbClient    *db.Client
	manager     *worker.Manager
	urlFetchers []urls.URLsFetcher
}

// Config holds configuration for the service
type Config struct {
	DBClient    *db.Client
	WorkerCount int
	MaxEntries  int
}

// NewService creates a new TextDownloadService
func NewService(config Config) *Service {
	mgr := worker.NewManager(config.WorkerCount, config.DBClient)

	// Initialize parsers in order: file, sitemap, then RSS
	parsers := []urls.URLsFetcher{
		urls.NewFileParser(),
		urls.NewSitemapParser(),
		urls.NewRSSParser(),
	}

	return &Service{
		dbClient:    config.DBClient,
		manager:     mgr,
		urlFetchers: parsers,
	}
}

// DownloadText downloads articles from the given URL (tries sitemap, then RSS)
func (s *Service) DownloadText(ctx context.Context, feedURL string, maxEntries int) error {
	var result []string
	var err error

	for i, fethcer := range s.urlFetchers {
		potentialUrls, fetchErr := fethcer.Fetch(feedURL)
		if fetchErr != nil {

			if i < len(s.urlFetchers)-1 {
				continue
			}
			// Last fetcher failed, return error
			return fmt.Errorf("all parsers failed, last error: %w", fetchErr)
		}

		if len(potentialUrls) == 0 {
			// Fetcher succeeded but no URLs found, try next parser
			if i < len(s.urlFetchers)-1 {
				continue
			}
			return fmt.Errorf("no URLs found in feed")
		}

	
		result = make([]string, 0, len(potentialUrls))
		for _, url := range potentialUrls {
			result = append(result, url.Location)
		}
		break
	}

	if len(result) == 0 {
		return fmt.Errorf("failed to parse feed from any parser")
	}

	// Get already-fetched URLs from database
	existingUrls, err := s.dbClient.GetAllURLs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get fetched URLs: %w", err)
	}

	// Apply filters
	filters := []urls.UrlFilter{
		urls.NewBaseURLFilter(),
		urls.NewAlreadyFetchedFilter(existingUrls),
	}

	filteredURLs, err := filterUrls(ctx, result, filters...)
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

// filterUrls applies all filters to a list of URLs
func filterUrls(ctx context.Context, urls []string, filters ...urls.UrlFilter) ([]string, error) {
	filtered := make([]string, 0, len(urls))

	for _, urlStr := range urls {
		keep := true
		for _, f := range filters {
			shouldKeep, err := f.ShouldKeep(ctx, urlStr)
			if err != nil {
				return nil, fmt.Errorf("filter error for URL %s: %w", urlStr, err)
			}
			if !shouldKeep {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, urlStr)
		}
	}

	return filtered, nil
}
