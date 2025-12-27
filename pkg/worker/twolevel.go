package worker

import (
	"context"
	"fmt"
	"log"
	"sync"

	"blog-search/pkg/db"
	"blog-search/pkg/urls"
)

// PageRange represents a range of pages to process (e.g., pages 1-10)
type PageRange struct {
	Start int // Starting page number (inclusive)
	End   int // Ending page number (inclusive)
}

// TwoLevelManager manages two levels of workers:
// Level 1: URL fetchers that extract URLs from paginated HTML pages
// Level 2: Content workers that fetch article content and save to MongoDB
type TwoLevelManager struct {
	urlFetcherWorkers int // Number of Level 1 workers (fetch URLs from pages)
	contentWorkers    int // Number of Level 2 workers (fetch content and save)
	dbClient          *db.Client
	pagesPerBatch     int
	baseURLPattern    string
	extractor         urls.URLExtractor
	maxPages          int // Maximum number of pages to process (0 = unlimited)
}

// Config holds configuration for TwoLevelManager
type Config struct {
	URLFetcherWorkers int
	ContentWorkers    int
	DBClient          *db.Client
	PagesPerBatch     int
	BaseURLPattern    string
	Extractor         urls.URLExtractor
	MaxPages          int // Maximum number of pages to process (0 = unlimited, useful for testing)
}

// NewTwoLevelManager creates a new two-level worker manager
func NewTwoLevelManager(config Config) *TwoLevelManager {
	return &TwoLevelManager{
		urlFetcherWorkers: config.URLFetcherWorkers,
		contentWorkers:    config.ContentWorkers,
		dbClient:          config.DBClient,
		pagesPerBatch:     config.PagesPerBatch,
		baseURLPattern:    config.BaseURLPattern,
		extractor:         config.Extractor,
		maxPages:          config.MaxPages,
	}
}

// ProcessPaginatedPages processes paginated HTML pages in batches
// Flow:
// 1. Manager generates page ranges and sends to pageRangeChan
// 2. Level 1 workers read page ranges, extract URLs, send to urlChan
// 3. Level 2 workers read URLs, fetch content, save to MongoDB
func (m *TwoLevelManager) ProcessPaginatedPages(ctx context.Context) error {
	// Create channels
	pageRangeChan := make(chan PageRange, m.urlFetcherWorkers*2) // Buffered channel for page ranges
	urlChan := make(chan string, m.contentWorkers*2)             // Buffered channel for article URLs

	// Start Level 2 workers first (content workers that save to MongoDB)
	var contentWg sync.WaitGroup
	m.startContentWorkers(ctx, &contentWg, urlChan)

	// Start Level 1 workers (URL fetchers)
	var urlFetcherWg sync.WaitGroup
	m.startURLFetcherWorkers(ctx, &urlFetcherWg, pageRangeChan, urlChan)

	// Manager generates page ranges and sends to pageRangeChan
	go m.generatePageRanges(ctx, pageRangeChan)

	// Wait for all URL fetcher workers to finish
	urlFetcherWg.Wait()
	close(urlChan) // Close URL channel when all URLs are extracted

	// Wait for all content workers to finish
	contentWg.Wait()

	return nil
}

// generatePageRanges generates page ranges and sends them to the channel
// Continues until a page returns no URLs (indicating end of pagination)
// or until MaxPages is reached (if set)
func (m *TwoLevelManager) generatePageRanges(ctx context.Context, pageRangeChan chan<- PageRange) {
	defer close(pageRangeChan)

	currentPage := 1
	htmlFetcher := urls.NewHTMLFetcher(m.extractor)
	pagesProcessed := 0

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check if we've reached the max pages limit
		if m.maxPages > 0 && pagesProcessed >= m.maxPages {
			log.Printf("Reached max pages limit (%d), stopping pagination", m.maxPages)
			return
		}

		// Check if the first page of this batch has URLs
		// If not, we've reached the end
		firstPageURL := fmt.Sprintf(m.baseURLPattern, currentPage)
		urls, err := htmlFetcher.Fetch(firstPageURL)
		if err != nil || len(urls) == 0 {
			// No URLs found, we've reached the end
			log.Printf("No URLs found at page %d, stopping pagination", currentPage)
			return
		}

		// Create a range for the current batch
		pageRange := PageRange{
			Start: currentPage,
			End:   currentPage + m.pagesPerBatch - 1,
		}

		// Send the range to workers
		select {
		case pageRangeChan <- pageRange:
			log.Printf("Generated page range: %d-%d", pageRange.Start, pageRange.End)
		case <-ctx.Done():
			return
		}

		// Move to next batch
		currentPage += m.pagesPerBatch
		pagesProcessed += m.pagesPerBatch
	}
}

// startURLFetcherWorkers starts Level 1 workers that:
// - Read PageRange from pageRangeChan
// - Fetch HTML from each page in the range
// - Extract article URLs from each page
// - Send each URL to urlChan
func (m *TwoLevelManager) startURLFetcherWorkers(ctx context.Context, wg *sync.WaitGroup, pageRangeChan <-chan PageRange, urlChan chan<- string) {
	for i := 0; i < m.urlFetcherWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			htmlFetcher := urls.NewHTMLFetcher(m.extractor)

			for {
				select {
				case pageRange, ok := <-pageRangeChan:
					if !ok {
						// Channel closed, no more ranges
						return
					}

					// Process this page range
					if err := m.processPageRange(ctx, workerID, pageRange, htmlFetcher, urlChan); err != nil {
						log.Printf("Worker %d: Error processing page range %d-%d: %v", workerID, pageRange.Start, pageRange.End, err)
					}

				case <-ctx.Done():
					return
				}
			}
		}(i)
	}
}

// startContentWorkers starts Level 2 workers that:
// - Read URLs from urlChan
// - Fetch article content from each URL
// - Save content to MongoDB
func (m *TwoLevelManager) startContentWorkers(ctx context.Context, wg *sync.WaitGroup, urlChan <-chan string) {
	for i := 0; i < m.contentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			contentWorker := NewWorker(m.dbClient)

			for {
				select {
				case url, ok := <-urlChan:
					if !ok {
						// Channel closed, no more URLs
						return
					}

					// Process this URL: fetch content and save to MongoDB
					if err := contentWorker.ProcessURL(ctx, url); err != nil {
						log.Printf("Content worker %d: Error processing URL %s: %v", workerID, url, err)
					} else {
						log.Printf("Content worker %d: Successfully processed %s", workerID, url)
					}

				case <-ctx.Done():
					return
				}
			}
		}(i)
	}
}

// processPageRange processes a single page range (used by Level 1 workers)
// Fetches URLs from all pages in the range and sends them to urlChan
func (m *TwoLevelManager) processPageRange(ctx context.Context, workerID int, pageRange PageRange, htmlFetcher *urls.HTMLFetcher, urlChan chan<- string) error {
	log.Printf("Worker %d: Processing page range %d-%d", workerID, pageRange.Start, pageRange.End)

	totalURLs := 0

	// Process each page in the range
	for pageNum := pageRange.Start; pageNum <= pageRange.End; pageNum++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch URLs from this page
		urls, err := m.fetchURLsFromPage(ctx, pageNum, htmlFetcher)
		if err != nil {
			// Log error but continue with next page
			log.Printf("Worker %d: Error fetching URLs from page %d: %v", workerID, pageNum, err)
			continue
		}

		// Send each URL to the channel
		for _, url := range urls {
			select {
			case urlChan <- url:
				totalURLs++
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	log.Printf("Worker %d: Extracted %d URLs from pages %d-%d", workerID, totalURLs, pageRange.Start, pageRange.End)
	return nil
}

// fetchURLsFromPage fetches URLs from a single page
func (m *TwoLevelManager) fetchURLsFromPage(ctx context.Context, pageNum int, htmlFetcher *urls.HTMLFetcher) ([]string, error) {
	// Build URL for this page
	pageURL := fmt.Sprintf(m.baseURLPattern, pageNum)

	// Fetch and extract URLs
	urls, err := htmlFetcher.Fetch(pageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URLs from page %d: %w", pageNum, err)
	}

	// Extract just the location strings
	result := make([]string, 0, len(urls))
	for _, u := range urls {
		if u.Location != "" {
			result = append(result, u.Location)
		}
	}

	return result, nil
}
