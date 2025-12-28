package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestSupabaseConnectionStringOnly tests connecting to Supabase using only
// a PostgreSQL connection string (no Supabase SDK).
// It verifies that we can read articles from the article table.
func TestSupabaseConnectionStringOnly(t *testing.T) {
	// Get connection string from environment variable
	connStr := os.Getenv("SUPABASE_CONNECTION_STRING")
	if connStr == "" {
		t.Skip("SUPABASE_CONNECTION_STRING environment variable not set, skipping test")
	}

	// Connect to Supabase using standard database/sql with pgx driver
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Test the connection
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Query articles from the article table
	query := `SELECT url, title, text, crawled_at FROM article LIMIT 10`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query articles: %v", err)
	}
	defer rows.Close()

	// Read and verify articles
	var articleCount int
	for rows.Next() {
		var url, title, text string
		var crawledAt time.Time

		if err := rows.Scan(&url, &title, &text, &crawledAt); err != nil {
			t.Fatalf("Failed to scan article row: %v", err)
		}

		// Verify we got valid data
		if url == "" {
			t.Error("Article URL is empty, expected non-empty URL")
		}

		articleCount++
		t.Logf("Article %d: URL=%s, Title=%s, Text length=%d, CrawledAt=%v",
			articleCount, url, title, len(text), crawledAt)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("Error iterating over rows: %v", err)
	}

	// Verify we can read articles (count >= 0, could be 0 if table is empty)
	t.Logf("Successfully read %d articles from Supabase", articleCount)
	if articleCount == 0 {
		t.Log("Warning: No articles found in the table (this is acceptable if the table is empty)")
	}
}

// TestSupabaseConnectionStringOnly_CountArticles tests counting articles in Supabase.
func TestSupabaseConnectionStringOnly_CountArticles(t *testing.T) {
	connStr := os.Getenv("SUPABASE_CONNECTION_STRING")
	if connStr == "" {
		t.Skip("SUPABASE_CONNECTION_STRING environment variable not set, skipping test")
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Count total articles
	var count int
	query := `SELECT COUNT(*) FROM article`
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		t.Fatalf("Failed to count articles: %v", err)
	}

	t.Logf("Total articles in Supabase: %d", count)
}

