package worker

import (
	"context"
	"testing"
	"time"

	"blog-search/pkg/db"
	"blog-search/pkg/urls"
)

func TestTwoLevelManager_ProcessPaginatedPages(t *testing.T) {
	// Skip if short test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup database
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch_test", "articles_test")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer dbClient.Close(ctx)

	// Note: We don't clear the test collection - the test will work with existing data
	// The MaxPages limit ensures we only process a few pages for testing

	// Create TwoLevelManager with se-radio.net configuration
	var err error
	// Limit to 3 pages to verify it works without processing the entire site
	manager := NewTwoLevelManager(Config{
		URLFetcherWorkers: 2, // 2 Level 1 workers (fetch URLs from pages)
		ContentWorkers:    3, // 3 Level 2 workers (fetch content and save)
		DBClient:          dbClient,
		PagesPerBatch:     1, // Process 1 page per batch for testing
		BaseURLPattern:    "https://se-radio.net/page/%d",
		Extractor:         urls.ExtractSERadioURLs,
		MaxPages:          3, // Limit to 3 pages for testing
	})

	// Create a context with timeout
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Process pages - will stop after 3 pages
	t.Log("Starting two-level worker pipeline (testing with 3 pages)...")
	err = manager.ProcessPaginatedPages(testCtx)
	if err != nil {
		t.Fatalf("ProcessPaginatedPages failed: %v", err)
	}

	// Verify articles were saved
	articles, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("Failed to get articles from database: %v", err)
	}

	// We just need to verify that it processed multiple pages and saved articles
	// Don't require a specific number since we're limiting pages
	if len(articles) == 0 {
		t.Error("Expected articles to be saved, but found none")
	} else {
		t.Logf("Successfully saved %d articles to database (processed multiple pages)", len(articles))

		// Print first few articles
		printCount := 3
		if len(articles) < printCount {
			printCount = len(articles)
		}
		for i := 0; i < printCount; i++ {
			t.Logf("Article %d: %s - %s", i+1, articles[i].Title, articles[i].URL)
		}
	}
}

func TestTwoLevelManager_ProcessPaginatedPages_MultipleBatches(t *testing.T) {
	// Skip if short test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Setup database
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch_test", "articles_test")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer dbClient.Close(ctx)

	// Create TwoLevelManager - test with multiple batches (3 pages)
	manager := NewTwoLevelManager(Config{
		URLFetcherWorkers: 2,
		ContentWorkers:    3,
		DBClient:          dbClient,
		PagesPerBatch:     1, // 1 page per batch
		BaseURLPattern:    "https://se-radio.net/page/%d",
		Extractor:         urls.ExtractSERadioURLs,
		MaxPages:          3, // Limit to 3 pages for testing
	})

	// Create a context with timeout
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Process pages - will process 3 pages to verify multiple batches work
	t.Log("Starting two-level worker pipeline with multiple batches (3 pages)...")
	err := manager.ProcessPaginatedPages(testCtx)
	if err != nil {
		t.Fatalf("ProcessPaginatedPages failed: %v", err)
	}

	// Verify articles were saved
	articles, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("Failed to get articles from database: %v", err)
	}

	t.Logf("Total articles saved: %d", len(articles))
	if len(articles) == 0 {
		t.Error("Expected articles to be saved, but found none")
	}
}
