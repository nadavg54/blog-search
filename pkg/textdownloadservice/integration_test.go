package textdownloadservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blog-search/pkg/db"
	"blog-search/pkg/domain"
)

func TestIntegration_ServiceDownload_WithFiltering(t *testing.T) {
	// Skip if short test
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	dbClient, ctx := setupDatabase(t)
	defer dbClient.Close(ctx)

	persistInitialArticles(t, ctx, dbClient)
	htmlServer, sitemapServer := createMockServers(t)

	service := createTestService(dbClient)
	u5URL := htmlServer.URL + "/u5"

	err := service.DownloadFromSitemap(ctx, sitemapServer.URL, 1000)
	if err != nil {
		t.Fatalf("Service failed to download: %v", err)
	}

	verifyFinalState(t, ctx, dbClient, []string{"u1", "u2", "u3"}, u5URL)
}

// setupDatabase creates and connects to a test database
func setupDatabase(t *testing.T) (*db.Client, context.Context) {
	t.Helper()

	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch_test", "articles_test")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	return dbClient, ctx
}

// persistInitialArticles saves the initial articles to the database
func persistInitialArticles(t *testing.T, ctx context.Context, dbClient *db.Client) {
	t.Helper()

	articles := []*domain.Article{
		{
			URL:       "u1",
			Title:     "Article 1",
			Text:      "Content 1",
			CrawledAt: time.Now(),
		},
		{
			URL:       "u2",
			Title:     "Article 2",
			Text:      "Content 2",
			CrawledAt: time.Now(),
		},
		{
			URL:       "u3",
			Title:     "Article 3",
			Text:      "Content 3",
			CrawledAt: time.Now(),
		},
	}

	for _, article := range articles {
		if err := dbClient.SaveArticle(ctx, article); err != nil {
			t.Fatalf("Failed to save article %s: %v", article.URL, err)
		}
	}

	// Verify initial state
	verifyURLsExist(t, ctx, dbClient, []string{"u1", "u2", "u3"})
}

// verifyURLsExist verifies that the given URLs exist in the database
func verifyURLsExist(t *testing.T, ctx context.Context, dbClient *db.Client, expectedURLs []string) {
	t.Helper()

	fetchedURLs, err := dbClient.GetAllURLs(ctx)
	if err != nil {
		t.Fatalf("Failed to get all URLs: %v", err)
	}

	expectedSet := make(map[string]bool)
	for _, url := range expectedURLs {
		expectedSet[url] = true
	}

	for _, url := range expectedURLs {
		if !fetchedURLs[url] {
			t.Errorf("Expected URL %s to be in fetched URLs", url)
		}
	}
}

// createMockServers creates mock HTTP servers for HTML content and sitemap
func createMockServers(t *testing.T) (*httptest.Server, *httptest.Server) {
	t.Helper()

	// Create HTML server that serves content for u5
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/u5" {
			w.Write([]byte(`<html><head><title>Article 5</title></head><body><article><h1>Article 5</h1><p>Content 5</p></article></body></html>`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Create sitemap server that includes u1, u2, and u5
	// The service should filter out u1 and u2 (already fetched) and only process u5
	sitemapXML := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>u1</loc>
	</url>
	<url>
		<loc>u2</loc>
	</url>
	<url>
		<loc>` + htmlServer.URL + `/u5</loc>
	</url>
</urlset>`

	sitemapServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(sitemapXML))
	}))

	return htmlServer, sitemapServer
}

// createTestService creates a service instance for testing
func createTestService(dbClient *db.Client) *Service {
	return NewService(Config{
		DBClient:    dbClient,
		WorkerCount: 1, // Use 1 worker for test simplicity
		MaxEntries:  1000,
	})
}

// verifyFinalState verifies the final state of URLs in the database
func verifyFinalState(t *testing.T, ctx context.Context, dbClient *db.Client, initialURLs []string, newURL string) {
	t.Helper()

	finalURLs, err := dbClient.GetAllURLs(ctx)
	if err != nil {
		t.Fatalf("Failed to get final URLs: %v", err)
	}

	// Verify we have at least the expected number of URLs
	expectedCount := len(initialURLs) + 1
	if len(finalURLs) < expectedCount {
		t.Errorf("Expected at least %d URLs in final state, got %d", expectedCount, len(finalURLs))
	}

	// Verify initial URLs are still there
	for _, url := range initialURLs {
		if !finalURLs[url] {
			t.Errorf("Expected URL %s to be in final URLs", url)
		}
	}

	// Verify new URL exists
	if !finalURLs[newURL] {
		t.Errorf("Expected %s to be in final URLs. Got: %v", newURL, finalURLs)
	}
}
