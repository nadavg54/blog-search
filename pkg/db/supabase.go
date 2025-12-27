package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	supabase "github.com/supabase-community/supabase-go"
)

// SupabaseConfig holds configuration required to connect to Supabase.
type SupabaseConfig struct {
	// ConnectionString is the Supabase Postgres connection string.
	// If not provided, will be constructed from SupabaseURL and Password.
	// Example: "postgresql://postgres:[password]@db.[project-ref].supabase.co:5432/postgres"
	ConnectionString string

	// SupabaseURL is the Supabase project URL (required if ConnectionString not provided).
	// Example: "https://[project-ref].supabase.co"
	SupabaseURL string

	// SupabaseKey is the Supabase API key (required for SDK features).
	// Use anon key for client-side, service_role key for server-side.
	SupabaseKey string

	// Password is the database password (required if ConnectionString not provided).
	// This is the database password, not the API key.
	Password string

	// Optional tuning knobs for database connection pool.
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxIdle  time.Duration
	ConnMaxLife  time.Duration
}

// SupabaseClient provides access to Supabase database and SDK features.
type SupabaseClient struct {
	db          *sql.DB
	supabaseSDK *supabase.Client
	cfg         SupabaseConfig
}

// NewSupabaseClient constructs a Supabase client.
func NewSupabaseClient(cfg SupabaseConfig) *SupabaseClient {
	return &SupabaseClient{cfg: cfg}
}

// Connect initializes the database connection and optionally the Supabase SDK client.
// If only URL and key are provided (no password/connection string), it will work in REST API mode only.
func (c *SupabaseClient) Connect(ctx context.Context) error {
	// Initialize Supabase SDK if URL and key are provided (required for REST API mode)
	if c.cfg.SupabaseURL != "" && c.cfg.SupabaseKey != "" {
		sdkClient, err := supabase.NewClient(c.cfg.SupabaseURL, c.cfg.SupabaseKey, nil)
		if err != nil {
			return fmt.Errorf("initialize supabase SDK: %w", err)
		}
		c.supabaseSDK = sdkClient
	}

	// Try to establish direct database connection if we have connection string or password
	connStr := c.cfg.ConnectionString
	if connStr == "" && c.cfg.Password != "" {
		var err error
		connStr, err = c.buildConnectionString()
		if err != nil {
			// If we can't build connection string but have SDK, continue in REST API mode
			if c.supabaseSDK != nil {
				return nil // REST API mode only
			}
			return fmt.Errorf("build connection string: %w", err)
		}
	}

	// If we have a connection string, connect to Postgres
	if connStr != "" {
		// Disable prepared statement cache and use simple protocol to avoid conflicts in parallel execution
		// Append parameters if not already present
		connStr = c.addConnectionParam(connStr, "statement_cache_capacity", "0")
		connStr = c.addConnectionParam(connStr, "default_query_exec_mode", "simple_protocol")

		db, err := sql.Open("pgx", connStr)
		if err != nil {
			// If direct connection fails but we have SDK, continue in REST API mode
			if c.supabaseSDK != nil {
				return nil // REST API mode only
			}
			return fmt.Errorf("open supabase postgres: %w", err)
		}

		// Apply optional pool tuning if provided.
		if c.cfg.MaxOpenConns > 0 {
			db.SetMaxOpenConns(c.cfg.MaxOpenConns)
		}
		if c.cfg.MaxIdleConns > 0 {
			db.SetMaxIdleConns(c.cfg.MaxIdleConns)
		}
		if c.cfg.ConnMaxIdle > 0 {
			db.SetConnMaxIdleTime(c.cfg.ConnMaxIdle)
		}
		if c.cfg.ConnMaxLife > 0 {
			db.SetConnMaxLifetime(c.cfg.ConnMaxLife)
		}

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			// If ping fails but we have SDK, continue in REST API mode
			if c.supabaseSDK != nil {
				return nil // REST API mode only
			}
			return fmt.Errorf("ping supabase postgres: %w", err)
		}

		c.db = db
	}

	// If we have neither DB connection nor SDK, that's an error
	if c.db == nil && c.supabaseSDK == nil {
		return fmt.Errorf("either connection string/password or Supabase URL+key must be provided")
	}

	return nil
}

// Close closes the database connection.
func (c *SupabaseClient) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB exposes the underlying sql.DB handle for direct database operations.
// Returns nil if only REST API mode is available (URL + key only, no password).
func (c *SupabaseClient) DB() *sql.DB {
	return c.db
}

// HasDirectDB returns true if direct database connection is available.
func (c *SupabaseClient) HasDirectDB() bool {
	return c.db != nil
}

// SDK returns the Supabase SDK client for accessing Supabase-specific features
// (auth, storage, real-time, etc.). Returns nil if SDK was not initialized.
func (c *SupabaseClient) SDK() *supabase.Client {
	return c.supabaseSDK
}

// buildConnectionString constructs a Supabase Postgres connection string from URL and password.
func (c *SupabaseClient) buildConnectionString() (string, error) {
	if c.cfg.SupabaseURL == "" {
		return "", fmt.Errorf("supabase URL is required when connection string is not provided")
	}
	if c.cfg.Password == "" {
		return "", fmt.Errorf("supabase password is required when connection string is not provided")
	}

	// Parse the Supabase URL to extract project reference
	// URL format: https://[project-ref].supabase.co
	parsedURL, err := url.Parse(c.cfg.SupabaseURL)
	if err != nil {
		return "", fmt.Errorf("parse supabase URL: %w", err)
	}

	host := parsedURL.Host
	// Extract project ref from host (e.g., "wmoiagolzzyhzkxthhvy.supabase.co" -> "wmoiagolzzyhzkxthhvy")
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid supabase URL format: expected [project-ref].supabase.co")
	}
	projectRef := parts[0]

	// Build connection string with SSL mode required for Supabase
	// URL encode the password to handle special characters
	encodedPassword := url.QueryEscape(c.cfg.Password)

	// Use direct connection with SSL
	// Disable prepared statement cache to avoid conflicts in parallel execution
	// statement_cache_capacity=0 disables the cache
	connStr := fmt.Sprintf("postgresql://postgres:%s@db.%s.supabase.co:5432/postgres?sslmode=require&statement_cache_capacity=0", encodedPassword, projectRef)

	return connStr, nil
}

// addConnectionParam adds a query parameter to the connection string if not already present.
func (c *SupabaseClient) addConnectionParam(connStr, key, value string) string {
	if strings.Contains(connStr, key+"=") {
		return connStr // Parameter already exists
	}

	separator := "?"
	if strings.Contains(connStr, "?") {
		separator = "&"
	}

	return connStr + separator + key + "=" + value
}
