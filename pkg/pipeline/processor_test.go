package pipeline

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

)


func TestHTTPContentProcessor_ProcessContent_HTTPError(t *testing.T) {
	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	processor := NewHTTPContentProcessor()
	ctx := context.Background()

	article, err := processor.ProcessContent(ctx, server.URL)

	if err == nil {
		t.Fatal("Expected error for 404 status, got nil")
	}

	if article != nil {
		t.Fatal("Expected nil article on error")
	}

	if !strings.Contains(err.Error(), "unexpected status code") {
		t.Errorf("Expected error about status code, got: %v", err)
	}
}

func TestHTTPContentProcessor_ProcessContent_EmptyResponse(t *testing.T) {
	// Create a test server that returns empty body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	}))
	defer server.Close()

	processor := NewHTTPContentProcessor()
	ctx := context.Background()

	article, err := processor.ProcessContent(ctx, server.URL)

	if err == nil {
		t.Fatal("Expected error for empty response, got nil")
	}

	if article != nil {
		t.Fatal("Expected nil article on error")
	}
}

func TestHTTPContentProcessor_ProcessContent_NotAcceptable(t *testing.T) {
	// Create a test server that returns "Not Acceptable" error page
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Not Acceptable"))
	}))
	defer server.Close()

	processor := NewHTTPContentProcessor()
	ctx := context.Background()

	article, err := processor.ProcessContent(ctx, server.URL)

	if err == nil {
		t.Fatal("Expected error for 'Not Acceptable' response, got nil")
	}

	if article != nil {
		t.Fatal("Expected nil article on error")
	}

	if !strings.Contains(err.Error(), "error or empty response") {
		t.Errorf("Expected error about error response, got: %v", err)
	}
}

// Note: DBContentSaver tests would require a real database connection or refactoring
// db.Client to use an interface. For now, DBContentSaver is a thin wrapper that
// delegates to db.Client.SaveArticle, so it's tested indirectly through integration tests.

func TestHTTPContentProcessor_ProcessContent_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a test server with realistic HTML
	htmlContent := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>SE Radio Episode 696: Flavia Saldanha on Data Engineering for AI</title>
		<meta property="og:title" content="SE Radio Episode 696: Flavia Saldanha on Data Engineering for AI" />
	</head>
	<body>
		<article>
			<header>
				<h1>SE Radio Episode 696: Flavia Saldanha on Data Engineering for AI</h1>
			</header>
			<div class="content">
				<p>In this episode, Flavia Saldanha discusses data engineering practices for AI systems.</p>
				<p>She covers topics such as data pipelines, feature stores, and MLOps.</p>
			</div>
		</article>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	processor := NewHTTPContentProcessor()
	ctx := context.Background()

	article, err := processor.ProcessContent(ctx, server.URL)

	if err != nil {
		t.Fatalf("ProcessContent failed: %v", err)
	}

	if article == nil {
		t.Fatal("ProcessContent returned nil article")
	}

	// Verify article fields
	if article.URL != server.URL {
		t.Errorf("Expected URL %s, got %s", server.URL, article.URL)
	}

	if !strings.Contains(article.Title, "Flavia Saldanha") {
		t.Errorf("Expected title to contain 'Flavia Saldanha', got: %s", article.Title)
	}

	if len(article.Text) == 0 {
		t.Error("Expected non-empty text content")
	}

	if article.CrawledAt.IsZero() {
		t.Error("Expected CrawledAt to be set")
	}

	// Verify text contains expected content
	if !strings.Contains(article.Text, "data engineering") {
		t.Error("Expected text to contain 'data engineering'")
	}
}
