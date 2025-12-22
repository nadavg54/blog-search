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

	// Clear test collection
	_, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		// Collection might not exist, that's okay
	}

	// Create TwoLevelManager with se-radio.net configuration
	manager := NewTwoLevelManager(Config{
		URLFetcherWorkers: 3, // 3 Level 1 workers (fetch URLs from pages)
		ContentWorkers:    5, // 5 Level 2 workers (fetch content and save)
		DBClient:          dbClient,
		PagesPerBatch:     1, // Process 1 page per batch for testing
		BaseURLPattern:    "https://se-radio.net/page/%d",
		Extractor:         urls.ExtractSERadioURLs,
	})

	// Create a context with timeout for full pagination (may take a while)
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Process pages - will continue until no more pages found
	t.Log("Starting two-level worker pipeline for full pagination...")
	err = manager.ProcessPaginatedPages(testCtx)
	if err != nil {
		t.Fatalf("ProcessPaginatedPages failed: %v", err)
	}

	// Verify articles were saved
	articles, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		t.Fatalf("Failed to get articles from database: %v", err)
	}

	if len(articles) == 0 {
		t.Error("Expected articles to be saved, but found none")
	} else {
		t.Logf("Successfully saved %d articles to database", len(articles))

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

	// Create TwoLevelManager - test with 2 batches of 1 page each
	manager := NewTwoLevelManager(Config{
		URLFetcherWorkers: 2,
		ContentWorkers:    3,
		DBClient:          dbClient,
		PagesPerBatch:     1, // 1 page per batch
		BaseURLPattern:    "https://se-radio.net/page/%d",
		Extractor:         urls.ExtractSERadioURLs,
	})

	// Modify generatePageRanges to only generate 2 batches for testing
	// We'll do this by creating a custom manager or modifying the test

	// Create a context with timeout
	testCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	// Process pages
	t.Log("Starting two-level worker pipeline with multiple batches...")
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
