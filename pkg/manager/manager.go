package manager

import (
	"context"
	"fmt"
	"log"
	"sync"

	"blog-search/pkg/db"
	"blog-search/pkg/worker"
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

	// Track results
	var successCount int64
	var errorCount int64
	var mu sync.Mutex

	// Start workers
	for i := 0; i < m.workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			w := worker.NewWorker(m.dbClient)

			// Process jobs from channel
			for url := range jobChan {
				err := w.ProcessURL(ctx, url)

				mu.Lock()
				if err != nil {
					errorCount++
					log.Printf("Worker %d: Error processing %s: %v", workerID, url, err)
				} else {
					successCount++
					if successCount%100 == 0 {
						log.Printf("Progress: %d successful, %d errors", successCount, errorCount)
					}
				}
				mu.Unlock()
			}
		}(i)
	}

	// Wait for all workers to finish
	wg.Wait()

	log.Printf("Completed: %d successful, %d errors (total: %d)", successCount, errorCount, len(urls))

	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all %d URLs failed to process", errorCount)
	}

	return nil
}
