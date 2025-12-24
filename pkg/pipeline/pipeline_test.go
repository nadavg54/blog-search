package pipeline

import (
	"context"
	"testing"
	"time"

	"blog-search/pkg/domain"
)

// mockURLGenerator is a mock implementation of URLGenerator for testing
type mockURLGenerator struct {
	urls []string
	err  error
}

func (m *mockURLGenerator) Generate(ctx context.Context) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.urls, nil
}

// mockURLFetcher is a mock implementation of URLFetcher for testing
type mockURLFetcher struct {
	urls map[string][]string // URL -> extracted URLs
	err  error
}

func (m *mockURLFetcher) Fetch(ctx context.Context, url string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if urls, ok := m.urls[url]; ok {
		return urls, nil
	}
	return []string{}, nil
}

// mockContentProcessor is a mock implementation of ContentProcessor for testing
type mockContentProcessor struct {
	articles  map[string]*domain.Article // URL -> Article
	err       error
	callCount int
}

func (m *mockContentProcessor) ProcessContent(ctx context.Context, url string) (*domain.Article, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	if article, ok := m.articles[url]; ok {
		return article, nil
	}
	// Return a default article if URL not in map
	return &domain.Article{
		URL:       url,
		Title:     "Test Article",
		Text:      "Test content",
		CrawledAt: time.Now(),
	}, nil
}

// mockContentSaver is a mock implementation of ContentSaver for testing
type mockContentSaver struct {
	savedArticles []*domain.Article
	err           error
	callCount     int
}

func (m *mockContentSaver) SaveArticle(ctx context.Context, article *domain.Article) error {
	m.callCount++
	if m.err != nil {
		return m.err
	}
	m.savedArticles = append(m.savedArticles, article)
	return nil
}

// Test Case 1: TestPipeline_Run_EmptySteps
// Input: Pipeline with 0 steps, Base URL: "https://example.com"
// Expected Output: Error "pipeline has no steps", Error is not nil
func TestPipeline_Run_EmptySteps(t *testing.T) {
	processor := &mockContentProcessor{
		articles: make(map[string]*domain.Article),
	}
	saver := &mockContentSaver{
		savedArticles: make([]*domain.Article, 0),
	}

	consumer := ContentConsumer{
		WorkerCount:      1,
		ContentProcessor: processor,
		ContentSaver:     saver,
	}

	pipeline := NewPipeline([]PipelineStep{}, consumer)
	ctx := context.Background()

	err := pipeline.Run(ctx, "https://example.com")

	if err == nil {
		t.Fatal("Expected error for empty steps, got nil")
	}

	if err.Error() != "pipeline has no steps" {
		t.Errorf("Expected error 'pipeline has no steps', got: %v", err)
	}
}

// Test Case 2: TestPipeline_Run_SingleStepWithGenerator
// Input:
//   - Pipeline with 1 step using Generator
//   - Generator returns: ["url1", "url2"]
//   - ContentProcessor processes URLs successfully
//   - ContentSaver saves successfully
//   - Base URL: "https://example.com"
//
// Expected Output:
//   - Success (no error)
//   - All URLs processed and saved
func TestPipeline_Run_SingleStepWithGenerator(t *testing.T) {
	// Setup mock Generator
	generator := &mockURLGenerator{
		urls: []string{
			"https://example.com/article1",
			"https://example.com/article2",
		},
	}

	// Setup mock ContentProcessor
	processor := &mockContentProcessor{
		articles: map[string]*domain.Article{
			"https://example.com/article1": {
				URL:       "https://example.com/article1",
				Title:     "Article 1",
				Text:      "Content 1",
				CrawledAt: time.Now(),
			},
			"https://example.com/article2": {
				URL:       "https://example.com/article2",
				Title:     "Article 2",
				Text:      "Content 2",
				CrawledAt: time.Now(),
			},
		},
	}

	// Setup mock ContentSaver
	saver := &mockContentSaver{
		savedArticles: make([]*domain.Article, 0),
	}

	// Create pipeline with single step using Generator
	step := PipelineStep{
		Name:        "Generator Step",
		WorkerCount: 1,
		Generator:   generator,
		Fetcher:     nil,
	}

	consumer := ContentConsumer{
		WorkerCount:      1,
		ContentProcessor: processor,
		ContentSaver:     saver,
	}

	pipeline := NewPipeline([]PipelineStep{step}, consumer)
	ctx := context.Background()

	// Run pipeline
	err := pipeline.Run(ctx, "https://example.com")

	// Verify no error
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify ContentProcessor was called for both URLs
	if processor.callCount != 2 {
		t.Errorf("Expected ContentProcessor.ProcessContent to be called 2 times, got %d", processor.callCount)
	}

	// Verify ContentSaver was called for both articles
	if saver.callCount != 2 {
		t.Errorf("Expected ContentSaver.SaveArticle to be called 2 times, got %d", saver.callCount)
	}

	// Verify both articles were saved
	if len(saver.savedArticles) != 2 {
		t.Fatalf("Expected 2 saved articles, got %d", len(saver.savedArticles))
	}

	// Verify saved articles match expected URLs
	savedURLs := make(map[string]bool)
	for _, article := range saver.savedArticles {
		savedURLs[article.URL] = true
	}

	if !savedURLs["https://example.com/article1"] {
		t.Error("Expected article1 to be saved")
	}

	if !savedURLs["https://example.com/article2"] {
		t.Error("Expected article2 to be saved")
	}
}

// Test Case 4: TestPipeline_Run_MultiStepPipeline
// Input:
//   - Pipeline with 2 steps:
//   - Step 1 (Generator): returns ["page1", "page2"]
//   - Step 2 (Fetcher): extracts URLs from each page
//   - ContentProcessor and ContentSaver work correctly
//   - Base URL: "https://example.com"
//
// Expected Output:
//   - Success (no error)
//   - URLs flow through both steps
//   - Final URLs processed and saved
func TestPipeline_Run_MultiStepPipeline(t *testing.T) {
	// Setup mock Generator (Step 1) - generates page URLs
	generator := &mockURLGenerator{
		urls: []string{
			"https://example.com/page1",
			"https://example.com/page2",
		},
	}

	// Setup mock Fetcher (Step 2) - extracts article URLs from page URLs
	fetcher := &mockURLFetcher{
		urls: map[string][]string{
			"https://example.com/page1": {
				"https://example.com/article1",
				"https://example.com/article2",
			},
			"https://example.com/page2": {
				"https://example.com/article3",
				"https://example.com/article4",
			},
		},
	}

	// Setup mock ContentProcessor
	processor := &mockContentProcessor{
		articles: map[string]*domain.Article{
			"https://example.com/article1": {
				URL:       "https://example.com/article1",
				Title:     "Article 1",
				Text:      "Content 1",
				CrawledAt: time.Now(),
			},
			"https://example.com/article2": {
				URL:       "https://example.com/article2",
				Title:     "Article 2",
				Text:      "Content 2",
				CrawledAt: time.Now(),
			},
			"https://example.com/article3": {
				URL:       "https://example.com/article3",
				Title:     "Article 3",
				Text:      "Content 3",
				CrawledAt: time.Now(),
			},
			"https://example.com/article4": {
				URL:       "https://example.com/article4",
				Title:     "Article 4",
				Text:      "Content 4",
				CrawledAt: time.Now(),
			},
		},
	}

	// Setup mock ContentSaver
	saver := &mockContentSaver{
		savedArticles: make([]*domain.Article, 0),
	}

	// Create pipeline with 2 steps
	step1 := PipelineStep{
		Name:        "Page Generator",
		WorkerCount: 1,
		Generator:   generator,
		Fetcher:     nil,
	}

	step2 := PipelineStep{
		Name:        "Article Fetcher",
		WorkerCount: 1,
		Generator:   nil,
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      1,
		ContentProcessor: processor,
		ContentSaver:     saver,
	}

	pipeline := NewPipeline([]PipelineStep{step1, step2}, consumer)
	ctx := context.Background()

	// Run pipeline
	err := pipeline.Run(ctx, "https://example.com")

	// Verify no error
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify ContentProcessor was called for all 4 article URLs
	expectedArticleCount := 4
	if processor.callCount != expectedArticleCount {
		t.Errorf("Expected ContentProcessor.ProcessContent to be called %d times, got %d", expectedArticleCount, processor.callCount)
	}

	// Verify ContentSaver was called for all 4 articles
	if saver.callCount != expectedArticleCount {
		t.Errorf("Expected ContentSaver.SaveArticle to be called %d times, got %d", expectedArticleCount, saver.callCount)
	}

	// Verify all 4 articles were saved
	if len(saver.savedArticles) != expectedArticleCount {
		t.Fatalf("Expected %d saved articles, got %d", expectedArticleCount, len(saver.savedArticles))
	}

	// Verify all expected article URLs were saved
	expectedURLs := map[string]bool{
		"https://example.com/article1": true,
		"https://example.com/article2": true,
		"https://example.com/article3": true,
		"https://example.com/article4": true,
	}

	savedURLs := make(map[string]bool)
	for _, article := range saver.savedArticles {
		savedURLs[article.URL] = true
	}

	for url := range expectedURLs {
		if !savedURLs[url] {
			t.Errorf("Expected article %s to be saved", url)
		}
	}

	// Verify we have exactly the expected URLs (no extras)
	if len(savedURLs) != len(expectedURLs) {
		t.Errorf("Expected exactly %d unique URLs saved, got %d", len(expectedURLs), len(savedURLs))
	}
}
