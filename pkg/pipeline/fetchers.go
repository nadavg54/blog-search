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
// If numberOfPages > 0, generates exactly that many pages without checking existence (except page 1 check)
// Implements URLGenerator interface
type PageRangeGenerator struct {
	baseURL             string                 // Base URL (e.g., "https://site.com")
	pagePattern         string                 // Page pattern with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
	pagesPerBatch       int                    // Not currently used, kept for backward compatibility
	numberOfPages       int                    // Number of pages to generate (0 = unlimited, check existence)
	httpClient          *httpclient.HTTPClient // Used to check if a page exists via HEAD request
	emptyContentMarkers []string               // Strings that indicate no content (e.g., "0 episodes found")
}

// NewPageRangeGenerator creates a new page range generator
// baseURL: the base URL (e.g., "https://site.com")
// pagePattern: the pattern for page URLs with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
// pagesPerBatch: not currently used, kept for backward compatibility
// numberOfPages: number of pages to generate (0 = unlimited, check existence for each page)
// extractor: not currently used, kept for backward compatibility (HEAD requests don't need content extraction)
func NewPageRangeGenerator(baseURL, pagePattern string, pagesPerBatch, numberOfPages int, extractor urls.URLExtractor) *PageRangeGenerator {
	return &PageRangeGenerator{
		baseURL:             baseURL,
		pagePattern:         pagePattern,
		pagesPerBatch:       pagesPerBatch,
		numberOfPages:       numberOfPages,
		httpClient:          httpclient.NewClient(httpclient.CloudflareClient),
		emptyContentMarkers: []string{"0 episodes found"}, // Default markers, can be extended
	}
}

// Generate generates page URLs from the configured pattern
// Returns page URLs that should be processed by the next step
// Stops when a page returns no URLs (indicating end of pagination)
// Automatically detects if page 1 should be the base URL (if /page/1 returns 404 or "not found")
// If numberOfPages > 0, generates exactly that many pages without checking existence (except page 1 check)
func (f *PageRangeGenerator) Generate(ctx context.Context) ([]string, error) {
	var allPageURLs []string

	// Always check if page 1 with pattern exists or contains "not found"
	// If not, use base URL for page 1 and start from page 2
	page1URL := f.buildPageURL(1)
	useBaseURLForPage1, err := f.shouldUseBaseURLForPage1(ctx, page1URL)
	if err != nil {
		log.Printf("PageRangeGenerator: Error checking page 1: %v - proceeding with pattern", err)
		useBaseURLForPage1 = false
	}

	var currentPage int
	if useBaseURLForPage1 {
		log.Printf("PageRangeGenerator: Page 1 with pattern not found, using base URL for page 1")
		allPageURLs = append(allPageURLs, f.baseURL)
		currentPage = 2
	} else {
		currentPage = 1
	}

	if f.numberOfPages > 0 {
		log.Printf("PageRangeGenerator: Generating exactly %d pages (existence checks bypassed)", f.numberOfPages)
	}

	// Single loop that handles both fixed page count and dynamic existence checking
	for {
		select {
		case <-ctx.Done():
			return allPageURLs, ctx.Err()
		default:
		}

		pageURL := f.buildPageURL(currentPage)
		shouldStop, err := f.shouldStopPagination(ctx, currentPage, pageURL, useBaseURLForPage1)
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

// shouldStopPagination checks if pagination should stop
// If numberOfPages > 0, stops when we've reached the target number of pages
// Otherwise, checks if the page exists and has content
func (f *PageRangeGenerator) shouldStopPagination(ctx context.Context, currentPage int, pageURL string, useBaseURLForPage1 bool) (bool, error) {
	// If numberOfPages is specified, check if we've reached the target
	if f.numberOfPages > 0 {
		if useBaseURLForPage1 {
			// We already added base URL as page 1, so count from page 2
			// If we're on page 2 and numberOfPages is 1, we should stop (already have 1 page)
			// If we're on page 3 and numberOfPages is 2, we should stop (already have 2 pages)
			// So: currentPage - 1 >= numberOfPages means we've generated enough
			if currentPage-1 >= f.numberOfPages {
				log.Printf("PageRangeGenerator: Reached target of %d pages - stopping pagination", f.numberOfPages)
				return true, nil
			}
		} else {
			// We start from page 1, so if currentPage > numberOfPages, we've generated enough
			if currentPage > f.numberOfPages {
				log.Printf("PageRangeGenerator: Reached target of %d pages - stopping pagination", f.numberOfPages)
				return true, nil
			}
		}
		return false, nil
	}

	// Otherwise, check existence for each page
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

// shouldUseBaseURLForPage1 checks if page 1 with pattern returns 404 or contains "not found"
// Returns true if base URL should be used for page 1, false otherwise
func (f *PageRangeGenerator) shouldUseBaseURLForPage1(ctx context.Context, page1URL string) (bool, error) {
	// First check if page exists
	exists, err := f.checkPageExists(page1URL)
	if err != nil {
		// Network error - assume page doesn't exist
		return true, nil
	}
	if !exists {
		// Page 1 with pattern doesn't exist (404), use base URL
		return true, nil
	}

	// Page exists, but check if it contains "not found" indicators
	hasContent, err := f.checkPageContentForNotFound(page1URL)
	if err != nil {
		// Error checking content - assume page is valid
		return false, nil
	}
	if !hasContent {
		// Page contains "not found" indicators, use base URL
		return true, nil
	}

	// Page 1 with pattern is valid, use it
	return false, nil
}

// checkPageContentForNotFound fetches the page content and checks if it contains "not found" indicators
// Returns true if content is valid, false if "not found" indicators found
func (f *PageRangeGenerator) checkPageContentForNotFound(pageURL string) (bool, error) {
	resp, err := f.httpClient.Get(pageURL)
	if err != nil {
		return false, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response body: %w", err)
	}

	bodyStr := strings.ToLower(string(body))

	// Check for "not found" indicators
	notFoundMarkers := []string{"not found", "404", "page not found", "nothing found", "no results found"}
	for _, marker := range notFoundMarkers {
		if strings.Contains(bodyStr, marker) {
			log.Printf("PageRangeGenerator: Found 'not found' marker '%s' in page 1", marker)
			return false, nil
		}
	}

	return true, nil
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

// PageCategoryGenerator generates category URLs from a base URL and a list of categories
// Used for sites where we need to generate URLs like "https://site.com/category1", "https://site.com/category2", etc.
// Implements URLGenerator interface
type PageCategoryGenerator struct {
	baseURL    string   // Base URL (e.g., "https://site.com")
	categories []string // List of categories to append to base URL
}

// NewPageCategoryGenerator creates a new page category generator
// baseURL: the base URL (e.g., "https://site.com")
// categories: list of categories to append to base URL (e.g., ["category1", "category2"])
func NewPageCategoryGenerator(baseURL string, categories []string) *PageCategoryGenerator {
	return &PageCategoryGenerator{
		baseURL:    baseURL,
		categories: categories,
	}
}

// Generate generates category URLs by appending each category to the base URL
// Returns all generated category URLs
func (f *PageCategoryGenerator) Generate(ctx context.Context) ([]string, error) {
	var categoryURLs []string

	for _, category := range f.categories {
		select {
		case <-ctx.Done():
			return categoryURLs, ctx.Err()
		default:
		}

		categoryURL := f.buildCategoryURL(category)
		categoryURLs = append(categoryURLs, categoryURL)
	}

	log.Printf("PageCategoryGenerator: Generated %d category URLs total", len(categoryURLs))
	return categoryURLs, nil
}

// buildCategoryURL builds the URL for a given category
func (f *PageCategoryGenerator) buildCategoryURL(category string) string {
	// Ensure base URL doesn't end with / and category doesn't start with /
	base := strings.TrimSuffix(f.baseURL, "/")
	cat := strings.TrimPrefix(category, "/")
	return base + "/" + cat
}
