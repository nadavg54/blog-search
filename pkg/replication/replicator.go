package replication

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"blog-search/pkg/db"
	"blog-search/pkg/domain"
)

// Config wires the replication dependencies.
type Config struct {
	Mongo    *db.Client
	Postgres *db.PostgresClient

	// Mongo collection name is currently baked into db.NewClient(..., collectionName).
	// We'll keep this out of config for now to match existing patterns.
}

// Replicator replicates data from MongoDB to Postgres.
//
// This is intentionally a one-shot, "copy everything" flow for now.
type Replicator struct {
	mongo *db.Client
	pg    *db.PostgresClient
}

func NewReplicator(cfg Config) (*Replicator, error) {
	if cfg.Mongo == nil {
		return nil, fmt.Errorf("mongo client is required")
	}
	if cfg.Postgres == nil {
		return nil, fmt.Errorf("postgres client is required")
	}
	return &Replicator{
		mongo: cfg.Mongo,
		pg:    cfg.Postgres,
	}, nil
}

// ReplicateArticlesMongoToPostgres reads all Articles from Mongo and inserts them
// into the Postgres `article` table.
//
// Behavior: if a URL already exists in Postgres, we skip inserting it.
// Processes articles in batches to avoid loading all URLs into memory at once.
func (r *Replicator) ReplicateArticlesMongoToPostgres(ctx context.Context) error {
	if err := r.ensureArticleSchema(ctx); err != nil {
		return err
	}

	articles, err := r.readAllArticlesFromMongo(ctx)
	if err != nil {
		return err
	}

	log.Printf("Loaded %d articles from Mongo, processing in batches...", len(articles))

	totalProcessed, totalInserted, err := r.processBatches(ctx, articles)
	if err != nil {
		return err
	}

	log.Printf("Replication complete: processed %d articles, inserted %d new articles", totalProcessed, totalInserted)
	return nil
}

// processBatches processes all articles in batches and returns total processed and inserted counts.
func (r *Replicator) processBatches(ctx context.Context, articles []domain.Article) (int, int, error) {
	const processBatchSize = 100
	totalProcessed := 0
	totalInserted := 0

	for start := 0; start < len(articles); start += processBatchSize {
		end := r.calculateBatchEnd(start, processBatchSize, len(articles))
		batch := articles[start:end]

		inserted, err := r.processBatch(ctx, batch, start, end)
		if err != nil {
			return totalProcessed, totalInserted, err
		}

		totalProcessed += len(batch)
		totalInserted += inserted
		r.logProgress(totalProcessed, len(articles), totalInserted, end >= len(articles))
	}

	return totalProcessed, totalInserted, nil
}

// calculateBatchEnd calculates the end index for a batch, ensuring it doesn't exceed the total length.
func (r *Replicator) calculateBatchEnd(start, batchSize, totalLen int) int {
	end := start + batchSize
	if end > totalLen {
		return totalLen
	}
	return end
}

// processBatch processes a single batch: checks existing URLs, filters new ones, and inserts them.
func (r *Replicator) processBatch(ctx context.Context, batch []domain.Article, start, end int) (int, error) {
	log.Printf("Processing batch [%d:%d] (%d articles)...", start, end, len(batch))

	existing, err := r.checkURLsExistInPostgres(ctx, batch)
	if err != nil {
		return 0, fmt.Errorf("check existing URLs for batch [%d:%d]: %w", start, end, err)
	}
	log.Printf("  Found %d existing URLs in Postgres", len(existing))

	toInsert := r.filterNewArticlesByURL(batch, existing)
	if len(toInsert) == 0 {
		log.Printf("  No new articles to insert")
		return 0, nil
	}

	log.Printf("  Inserting %d new articles...", len(toInsert))
	if err := r.insertArticlesTx(ctx, toInsert); err != nil {
		return 0, fmt.Errorf("insert batch [%d:%d]: %w", start, end, err)
	}
	log.Printf("  ✓ Inserted %d articles", len(toInsert))

	return len(toInsert), nil
}

// logProgress logs progress at regular intervals or at completion.
func (r *Replicator) logProgress(processed, total, inserted int, isComplete bool) {
	if processed%1000 == 0 || isComplete {
		log.Printf("Progress: processed %d/%d articles, inserted %d new articles", processed, total, inserted)
	}
}

func (r *Replicator) ensureArticleSchema(ctx context.Context) error {
	if r.pg.DB() == nil {
		return fmt.Errorf("postgres DB not connected")
	}

	// Keep schema simple: url is the primary key, which also gives us uniqueness.
	//
	// NOTE: we default crawled_at to now() so older Mongo docs missing crawled_at
	// can still be inserted (implementation still sets it explicitly when present).
	const ddl = `
CREATE TABLE IF NOT EXISTS article (
  url TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL DEFAULT '',
  crawled_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

	if _, err := r.pg.DB().ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create article table: %w", err)
	}
	return nil
}

// checkURLsExistInPostgres checks which URLs from the given batch already exist in Postgres.
// This avoids loading all URLs into memory at once.
func (r *Replicator) checkURLsExistInPostgres(ctx context.Context, batch []domain.Article) (map[string]bool, error) {
	if r.pg.DB() == nil {
		return nil, fmt.Errorf("postgres DB not connected")
	}
	if len(batch) == 0 {
		return map[string]bool{}, nil
	}

	urls := r.extractURLsFromBatch(batch)
	if len(urls) == 0 {
		return map[string]bool{}, nil
	}

	query, args := r.buildURLInQuery(urls)
	return r.executeURLQuery(ctx, query, args)
}

// extractURLsFromBatch extracts non-empty URLs from a batch of articles.
func (r *Replicator) extractURLsFromBatch(batch []domain.Article) []interface{} {
	urls := make([]interface{}, 0, len(batch))
	for _, a := range batch {
		if a.URL != "" {
			urls = append(urls, a.URL)
		}
	}
	return urls
}

// buildURLInQuery builds a SQL query with IN clause and returns the query string and arguments.
func (r *Replicator) buildURLInQuery(urls []interface{}) (string, []interface{}) {
	query := `SELECT url FROM article WHERE url IN (`
	args := make([]interface{}, len(urls))
	for i, url := range urls {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = url
	}
	query += ")"
	return query, args
}

// executeURLQuery executes a URL query and returns the results as a set.
func (r *Replicator) executeURLQuery(ctx context.Context, query string, args []interface{}) (map[string]bool, error) {
	rows, err := r.pg.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query existing urls: %w", err)
	}
	defer rows.Close()

	set := make(map[string]bool)
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, fmt.Errorf("scan url: %w", err)
		}
		if url != "" {
			set[url] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return set, nil
}

func (r *Replicator) readAllArticlesFromMongo(ctx context.Context) ([]domain.Article, error) {
	articles, err := r.mongo.GetAllArticles(ctx)
	if err != nil {
		return nil, err
	}
	return articles, nil
}

func (r *Replicator) filterNewArticlesByURL(all []domain.Article, existing map[string]bool) []domain.Article {
	if existing == nil {
		existing = map[string]bool{}
	}

	out := make([]domain.Article, 0, len(all))
	for _, a := range all {
		if a.URL == "" {
			continue
		}
		if existing[a.URL] {
			continue
		}
		out = append(out, a)
	}
	return out
}

// insertArticlesTx inserts a batch of articles within a transaction.
func (r *Replicator) insertArticlesTx(ctx context.Context, batch []domain.Article) error {
	tx, err := r.pg.DB().BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := r.executeBatchInsert(ctx, tx, batch); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// executeBatchInsert executes the insert statements for a batch of articles.
func (r *Replicator) executeBatchInsert(ctx context.Context, tx *sql.Tx, batch []domain.Article) error {
	const insertQuery = `
INSERT INTO article (url, title, text, crawled_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (url) DO NOTHING`

	stmt, err := tx.PrepareContext(ctx, insertQuery)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, a := range batch {
		if a.URL == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, a.URL, a.Title, a.Text, a.CrawledAt); err != nil {
			return fmt.Errorf("insert article url=%q: %w", a.URL, err)
		}
	}

	return nil
}
