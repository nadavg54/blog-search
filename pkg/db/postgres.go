package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresConfig holds configuration required to connect to Postgres.
//
// Keep this similar in spirit to the Mongo client config style used today
// (explicit connection string passed from main).
type PostgresConfig struct {
	// DSN example (lib/pq style):
	// "postgres://user:pass@localhost:5432/blogsearch?sslmode=disable"
	DSN string

	// Optional tuning knobs for later.
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxIdle  time.Duration
	ConnMaxLife  time.Duration
}

// PostgresClient is a thin wrapper around a sql.DB handle.
//
// NOTE: This is intentionally a skeleton. We'll add a concrete driver (pgx or lib/pq)
// and the real connect/open logic after you ask for full implementation.
type PostgresClient struct {
	db  *sql.DB
	cfg PostgresConfig
}

// NewPostgresClient constructs a Postgres client skeleton.
func NewPostgresClient(cfg PostgresConfig) *PostgresClient {
	return &PostgresClient{cfg: cfg}
}

// Connect initializes the underlying sql.DB handle and verifies connectivity.
func (c *PostgresClient) Connect(ctx context.Context) error {
	if c.cfg.DSN == "" {
		return fmt.Errorf("postgres DSN is required")
	}

	db, err := sql.Open("pgx", c.cfg.DSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
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
		return fmt.Errorf("ping postgres: %w", err)
	}

	c.db = db
	return nil
}

// Close closes the underlying sql.DB handle.
func (c *PostgresClient) Close() error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB exposes the underlying handle for query/exec operations.
func (c *PostgresClient) DB() *sql.DB {
	return c.db
}
