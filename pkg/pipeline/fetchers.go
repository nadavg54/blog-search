package pipeline

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"blog-search/pkg/httpclient"
	"blog-search/pkg/urls"
)

// BasicUrlFetcher wraps a URLsFetcher to extract URLs from a base URL
// Used for RSS, Sitemap, etc. where we extract URLs directly from base URL
type BasicUrlFetcher struct {
	fetcher urls.URLsFetcher
	filters []urls.UrlFilter
}

// NewBasicURLFetcher creates a new base URL fetcher
func NewBasicURLFetcher(fetcher urls.URLsFetcher) *BasicUrlFetcher {
	return &BasicUrlFetcher{
		fetcher: fetcher,
		filters: nil,
	}
}

// NewBasicURLFetcherWithFilters creates a new base URL fetcher with filters
func NewBasicURLFetcherWithFilters(fetcher urls.URLsFetcher, filters []urls.UrlFilter) *BasicUrlFetcher {
	return &BasicUrlFetcher{
		fetcher: fetcher,
		filters: filters,
	}
}

// Fetch extracts URLs from the given base URL and applies filters
func (f *BasicUrlFetcher) Fetch(ctx context.Context, baseURL string) ([]string, error) {
	log.Printf("BasicUrlFetcher: Fetching URLs from %s", baseURL)
	urls, err := f.fetchURLs(baseURL)
	if err != nil {
		return nil, err
	}

	result := f.extractLocations(urls)

	// Apply filters if any
	if len(f.filters) > 0 {
		filtered, err := f.applyFilters(ctx, result)
		if err != nil {
			return nil, err
		}
		result = filtered
	}

	log.Printf("BasicUrlFetcher: Returning %d URLs", len(result))
	return result, nil
}

// fetchURLs fetches URLs from the underlying fetcher
func (f *BasicUrlFetcher) fetchURLs(baseURL string) ([]urls.URL, error) {
	urls, err := f.fetcher.Fetch(baseURL)
	if err != nil {
		log.Printf("BasicUrlFetcher: ERROR fetching URLs from %s: %v", baseURL, err)
		return nil, fmt.Errorf("failed to fetch URLs: %w", err)
	}
	log.Printf("BasicUrlFetcher: Fetched %d URLs from %s", len(urls), baseURL)
	return urls, nil
}

// extractLocations extracts location strings from URL structs
func (f *BasicUrlFetcher) extractLocations(urls []urls.URL) []string {
	result := make([]string, 0, len(urls))
	for _, u := range urls {
		if u.Location != "" {
			result = append(result, u.Location)
		}
	}
	log.Printf("BasicUrlFetcher: Extracted %d URLs with non-empty Location", len(result))
	return result
}

// applyFilters applies URL filters to the result set
func (f *BasicUrlFetcher) applyFilters(ctx context.Context, result []string) ([]string, error) {
	if len(f.filters) == 0 {
		return result, nil
	}

	filtered := make([]string, 0, len(result))
	for _, urlStr := range result {
		shouldKeep, err := f.shouldKeepURL(ctx, urlStr)
		if err != nil {
			return nil, fmt.Errorf("filter error: %w", err)
		}
		if shouldKeep {
			filtered = append(filtered, urlStr)
		}
	}
	return filtered, nil
}

// shouldKeepURL checks if a URL should be kept after applying all filters
// Returns (shouldKeep, error) - error is returned if any filter fails
func (f *BasicUrlFetcher) shouldKeepURL(ctx context.Context, urlStr string) (bool, error) {
	for _, filter := range f.filters {
		keep, err := filter.ShouldKeep(ctx, urlStr)
		if err != nil {
			return false, err
		}
		if !keep {
			return false, nil
		}
	}
	return true, nil
}

// NewHTMLPageFetcher creates a BasicUrlFetcher for HTML pages
// This is a convenience function that wraps HTMLFetcher (which implements URLsFetcher)
func NewHTMLPageFetcher(extractor urls.URLExtractor) *BasicUrlFetcher {
	return NewBasicURLFetcher(urls.NewHTMLFetcher(extractor))
}

// NewHTMLPageFetcherWithFilters creates a BasicUrlFetcher for HTML pages with filters
// This is a convenience function that wraps HTMLFetcher (which implements URLsFetcher)
func NewHTMLPageFetcherWithFilters(extractor urls.URLExtractor, filters []urls.UrlFilter) *BasicUrlFetcher {
	return NewBasicURLFetcherWithFilters(urls.NewHTMLFetcher(extractor), filters)
}

// PageRangeGenerator generates page URLs from a base URL and page pattern
// Used for paginated sites where we need to generate URLs like "https://site.com/page/1", "page/2", etc.
// It generates page URLs until it finds a page that doesn't exist (404 or other error)
// or contains content indicating no more pages (e.g., "0 episodes found")
// Implements URLGenerator interface
type PageRangeGenerator struct {
	baseURL             string                 // Base URL (e.g., "https://site.com")
	pagePattern         string                 // Page pattern with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
	pagesPerBatch       int                    // Not currently used, kept for backward compatibility
	httpClient          *httpclient.HTTPClient // Used to check if a page exists via HEAD request
	emptyContentMarkers []string               // Strings that indicate no content (e.g., "0 episodes found")
}

// NewPageRangeGenerator creates a new page range generator
// baseURL: the base URL (e.g., "https://site.com")
// pagePattern: the pattern for page URLs with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
// pagesPerBatch: not currently used, kept for backward compatibility
// extractor: not currently used, kept for backward compatibility (HEAD requests don't need content extraction)
func NewPageRangeGenerator(baseURL, pagePattern string, pagesPerBatch int, extractor urls.URLExtractor) *PageRangeGenerator {
	return &PageRangeGenerator{
		baseURL:             baseURL,
		pagePattern:         pagePattern,
		pagesPerBatch:       pagesPerBatch,
		httpClient:          httpclient.NewClient(httpclient.CloudflareClient),
		emptyContentMarkers: []string{"0 episodes found"}, // Default markers, can be extended
	}
}

// Generate generates page URLs from the configured pattern
// Returns page URLs that should be processed by the next step
// Stops when a page returns no URLs (indicating end of pagination)
func (f *PageRangeGenerator) Generate(ctx context.Context) ([]string, error) {
	var allPageURLs []string
	currentPage := 1

	for {
		select {
		case <-ctx.Done():
			return allPageURLs, ctx.Err()
		default:
		}

		pageURL := f.buildPageURL(currentPage)
		shouldStop, err := f.shouldStopPagination(ctx, currentPage, pageURL)
		if err != nil || shouldStop {
			break
		}

		allPageURLs = append(allPageURLs, pageURL)
		currentPage++
	}

	log.Printf("PageRangeGenerator: Generated %d page URLs total", len(allPageURLs))
	return allPageURLs, nil
}

// buildPageURL builds the URL for a given page number
func (f *PageRangeGenerator) buildPageURL(pageNum int) string {
	return f.baseURL + fmt.Sprintf(f.pagePattern, pageNum)
}

// shouldStopPagination checks if pagination should stop by checking if the page exists and has content
func (f *PageRangeGenerator) shouldStopPagination(ctx context.Context, currentPage int, pageURL string) (bool, error) {
	exists, err := f.checkPageExists(pageURL)
	if err != nil {
		log.Printf("PageRangeGenerator: Error checking page %d: %v - stopping pagination", currentPage, err)
		return true, err
	}
	if !exists {
		log.Printf("PageRangeGenerator: Page %d does not exist - stopping pagination", currentPage)
		return true, nil
	}

	// Every 10 pages, check content for empty markers
	if currentPage%10 == 0 {
		return f.shouldStopDueToEmptyContent(ctx, currentPage, pageURL)
	}

	return false, nil
}

// checkPageExists checks if a page exists using a HEAD request
func (f *PageRangeGenerator) checkPageExists(pageURL string) (bool, error) {
	log.Printf("PageRangeGenerator: Checking page: %s", pageURL)
	resp, err := f.httpClient.Head(pageURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	log.Printf("PageRangeGenerator: Page exists (status %d)", resp.StatusCode)
	return true, nil
}

// shouldStopDueToEmptyContent checks if pagination should stop due to empty content markers
func (f *PageRangeGenerator) shouldStopDueToEmptyContent(ctx context.Context, currentPage int, pageURL string) (bool, error) {
	log.Printf("PageRangeGenerator: Page %d is a multiple of 10, checking content for empty markers", currentPage)
	hasContent, err := f.checkPageContent(pageURL)
	if err != nil {
		log.Printf("PageRangeGenerator: Error checking content for page %d: %v - continuing", currentPage, err)
		return false, nil // Continue on error
	}
	if !hasContent {
		log.Printf("PageRangeGenerator: Page %d contains empty content markers - stopping pagination", currentPage)
		return true, nil
	}
	return false, nil
}

// checkPageContent fetches the page content and checks if it contains empty content markers
// Returns true if content exists, false if empty markers found
func (f *PageRangeGenerator) checkPageContent(pageURL string) (bool, error) {
	resp, err := f.httpClient.Get(pageURL)
	if err != nil {
		return false, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %w", err)
	}

	bodyStr := strings.ToLower(string(body))

	// Check for empty content markers
	for _, marker := range f.emptyContentMarkers {
		if strings.Contains(bodyStr, strings.ToLower(marker)) {
			log.Printf("PageRangeGenerator: Found empty content marker '%s' in page", marker)
			return false, nil
		}
	}

	return true, nil
}
