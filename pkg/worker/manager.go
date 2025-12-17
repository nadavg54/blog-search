package worker

import (
	"context"
	"fmt"
	"log"
	"sync"

	"blog-search/pkg/db"
)

// Manager manages workers and distributes URLs to them
type Manager struct {
	workerCount int
	dbClient    *db.Client
}

// NewManager creates a new manager
func NewManager(workerCount int, dbClient *db.Client) *Manager {
	return &Manager{
		workerCount: workerCount,
		dbClient:    dbClient,
	}
}

// ProcessURLs distributes URLs to workers and processes them concurrently
func (m *Manager) ProcessURLs(ctx context.Context, urls []string) error {
	// Create job channel
	jobChan := make(chan string, len(urls))

	// Send all URLs to job channel
	for _, url := range urls {
		jobChan <- url
	}
	close(jobChan)

	// Create wait group to wait for all workers
	var wg sync.WaitGroup

	// Results channel to collect success/error from workers (no contention)
	type result struct {
		success  bool
		url      string
		workerID int
		err      error
	}
	resultsChan := make(chan result, len(urls))

	// Start workers
	for i := 0; i < m.workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			w := NewWorker(m.dbClient)

			// Process jobs from channel - each worker tracks its own counts
			for url := range jobChan {
				err := w.ProcessURL(ctx, url)

				// Send result to channel (no contention during processing)
				resultsChan <- result{
					success:  err == nil,
					url:      url,
					workerID: workerID,
					err:      err,
				}
			}
		}(i)
	}

	// Close results channel when all workers finish
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Aggregate results (no contention - single goroutine reads from channel)
	var successCount, errorCount uint64

	for res := range resultsChan {
		if res.success {
			successCount++
			// Log progress every 100 successful
			if successCount%100 == 0 {
				log.Printf("Progress: %d successful, %d errors", successCount, errorCount)
			}
		} else {
			errorCount++
			log.Printf("Worker %d: Error processing %s: %v", res.workerID, res.url, res.err)
		}
	}

	log.Printf("Completed: %d successful, %d errors (total: %d)", successCount, errorCount, len(urls))

	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all %d URLs failed to process", errorCount)
	}

	return nil
}
