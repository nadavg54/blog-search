package pipeline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"blog-search/pkg/urls"
)

// mockURLsFetcher is a mock implementation of urls.URLsFetcher for testing
type mockURLsFetcher struct {
	urls []urls.URL
	err  error
}

func (m *mockURLsFetcher) Fetch(baseURL string) ([]urls.URL, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.urls, nil
}

// mockUrlFilter is a mock implementation of urls.UrlFilter for testing
type mockUrlFilter struct {
	shouldKeep bool
	err        error
}

func (m *mockUrlFilter) ShouldKeep(ctx context.Context, url string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.shouldKeep, nil
}

func TestBasicUrlFetcher_Fetch_Success(t *testing.T) {
	mockFetcher := &mockURLsFetcher{
		urls: []urls.URL{
			{Location: "https://example.com/article1", Title: "Article 1"},
			{Location: "https://example.com/article2", Title: "Article 2"},
			{Location: "", Title: "Empty Location"}, // Should be filtered out
		},
	}

	fetcher := NewBasicURLFetcher(mockFetcher)
	ctx := context.Background()

	result, err := fetcher.Fetch(ctx, "https://example.com")

	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(result))
	}

	if result[0] != "https://example.com/article1" {
		t.Errorf("Expected first URL to be 'https://example.com/article1', got '%s'", result[0])
	}

	if result[1] != "https://example.com/article2" {
		t.Errorf("Expected second URL to be 'https://example.com/article2', got '%s'", result[1])
	}
}

func TestBasicUrlFetcher_Fetch_WithFilters(t *testing.T) {
	mockFetcher := &mockURLsFetcher{
		urls: []urls.URL{
			{Location: "https://example.com/article1", Title: "Article 1"},
			{Location: "https://example.com/article2", Title: "Article 2"},
			{Location: "https://example.com/article3", Title: "Article 3"},
		},
	}

	// Filter with configurable shouldKeep behavior
	filter := &mockUrlFilter{
		shouldKeep: false, // Will be set per test call
	}

	fetcher := NewBasicURLFetcherWithFilters(mockFetcher, []urls.UrlFilter{filter})
	ctx := context.Background()

	// First call - filter rejects all URLs
	filter.shouldKeep = false
	result, err := fetcher.Fetch(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 URLs after filter rejects all, got %d", len(result))
	}

	// Second call - filter accepts all URLs
	filter.shouldKeep = true
	result, err = fetcher.Fetch(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 URLs after filter accepts all, got %d", len(result))
	}
}

func TestBasicUrlFetcher_Fetch_FilterError(t *testing.T) {
	mockFetcher := &mockURLsFetcher{
		urls: []urls.URL{
			{Location: "https://example.com/article1", Title: "Article 1"},
		},
	}

	expectedError := errors.New("filter error")
	filter := &mockUrlFilter{
		err: expectedError,
	}

	fetcher := NewBasicURLFetcherWithFilters(mockFetcher, []urls.UrlFilter{filter})
	ctx := context.Background()

	result, err := fetcher.Fetch(ctx, "https://example.com")

	if err == nil {
		t.Fatal("Expected error from filter, got nil")
	}

	if result != nil {
		t.Fatal("Expected nil result on filter error")
	}

	if !errors.Is(err, expectedError) && err.Error() != "filter error: "+expectedError.Error() {
		t.Errorf("Expected filter error, got: %v", err)
	}
}

func TestBasicUrlFetcher_Fetch_UnderlyingFetcherError(t *testing.T) {
	expectedError := errors.New("network error")
	mockFetcher := &mockURLsFetcher{
		err: expectedError,
	}

	fetcher := NewBasicURLFetcher(mockFetcher)
	ctx := context.Background()

	result, err := fetcher.Fetch(ctx, "https://example.com")

	if err == nil {
		t.Fatal("Expected error from underlying fetcher, got nil")
	}

	if result != nil {
		t.Fatal("Expected nil result on error")
	}

	if !errors.Is(err, expectedError) && !errors.Is(err, errors.Unwrap(err)) {
		t.Errorf("Expected wrapped error, got: %v", err)
	}
}

func TestPageRangeGenerator_Generate_Success(t *testing.T) {
	// Create a test server that returns 200 for pages 1-3, then 404
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 3 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	generator := NewPageRangeGenerator(server.URL, "/page/%d", 10, nil)
	ctx := context.Background()

	result, err := generator.Generate(ctx)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("Expected 3 page URLs, got %d", len(result))
	}

	expectedURL1 := server.URL + "/page/1"
	if result[0] != expectedURL1 {
		t.Errorf("Expected first URL to be '%s', got '%s'", expectedURL1, result[0])
	}

	expectedURL3 := server.URL + "/page/3"
	if result[2] != expectedURL3 {
		t.Errorf("Expected third URL to be '%s', got '%s'", expectedURL3, result[2])
	}
}

func TestPageRangeGenerator_Generate_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	generator := NewPageRangeGenerator(server.URL, "/page/%d", 10, nil)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context immediately
	cancel()

	result, err := generator.Generate(ctx)

	if err == nil {
		t.Fatal("Expected error from cancelled context, got nil")
	}

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}

	// Should return empty result (nil slice is valid in Go and has len 0)
	if len(result) != 0 {
		t.Errorf("Expected empty result on immediate cancellation, got %d URLs", len(result))
	}
}

func TestPageRangeGenerator_Generate_NoPages(t *testing.T) {
	// Server returns 404 immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	generator := NewPageRangeGenerator(server.URL, "/page/%d", 10, nil)
	ctx := context.Background()

	result, err := generator.Generate(ctx)

	if err != nil {
		t.Fatalf("Generate should not error on no pages, got: %v", err)
	}

	if len(result) != 0 {
		t.Fatalf("Expected 0 page URLs, got %d", len(result))
	}
}

func TestNewHTMLPageFetcher(t *testing.T) {
	extractor := func(html string) ([]urls.URL, error) {
		return []urls.URL{{Location: "https://example.com/article"}}, nil
	}

	fetcher := NewHTMLPageFetcher(extractor)

	if fetcher == nil {
		t.Fatal("NewHTMLPageFetcher returned nil")
	}

	if fetcher.fetcher == nil {
		t.Fatal("HTMLPageFetcher's underlying fetcher is nil")
	}
}

func TestNewHTMLPageFetcherWithFilters(t *testing.T) {
	extractor := func(html string) ([]urls.URL, error) {
		return []urls.URL{{Location: "https://example.com/article"}}, nil
	}

	filter := &mockUrlFilter{shouldKeep: true}
	fetcher := NewHTMLPageFetcherWithFilters(extractor, []urls.UrlFilter{filter})

	if fetcher == nil {
		t.Fatal("NewHTMLPageFetcherWithFilters returned nil")
	}

	if fetcher.fetcher == nil {
		t.Fatal("HTMLPageFetcher's underlying fetcher is nil")
	}

	if len(fetcher.filters) != 1 {
		t.Fatalf("Expected 1 filter, got %d", len(fetcher.filters))
	}
}
