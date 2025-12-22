package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"blog-search/pkg/db"
	"blog-search/pkg/replication"
	"blog-search/pkg/textdownloadservice"
	"blog-search/pkg/urls"
	"blog-search/pkg/worker"
)

func main() {
	// Subcommand: replicate (one-shot Mongo -> Postgres)
	//
	// Example:
	//   MONGO_URI="mongodb://admin:password@localhost:27017" \
	//   POSTGRES_DSN="postgres://user:pass@localhost:5432/blogsearch?sslmode=disable" \
	//   go run . replicate
	if len(os.Args) > 1 && os.Args[1] == "replicate" {
		runReplication()
		return
	}

	// Subcommand: paginate (fetch from paginated HTML pages)
	//
	// Example:
	//   MONGO_URI="mongodb://admin:password@localhost:27017" \
	//   go run . paginate
	//
	// Or with custom configuration:
	//   go run . paginate https://se-radio.net/page/%d 10 3 5
	//   (baseURLPattern, pagesPerBatch, urlFetcherWorkers, contentWorkers)
	if len(os.Args) > 1 && os.Args[1] == "paginate" {
		runPaginatedFetch()
		return
	}

	// Get sitemap URL from command line or use default
	sitemapURL := "https://rss.libsyn.com/shows/21070/destinations/23379.xml"

	if len(os.Args) > 1 {
		sitemapURL = os.Args[1]
	}

	// Initialize database client
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch", "articles")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	// Create service
	service := textdownloadservice.NewService(textdownloadservice.Config{
		DBClient:    dbClient,
		WorkerCount: 50,
		MaxEntries:  10000,
	})

	// Download articles from sitemap using the service
	log.Printf("Processing articles from sitemap: %s", sitemapURL)
	if err := service.DownloadText(ctx, sitemapURL, 10000); err != nil {
		log.Fatalf("Failed to download articles: %v", err)
	}

	log.Println("All done!")
}

func runReplication() {
	ctx := context.Background()

	// Keep config style similar to existing Mongo usage: explicit connection strings.
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://admin:password@localhost:27017"
	}

	postgresDSN := os.Getenv("POSTGRES_DSN")

	mongo := db.NewClient(mongoURI, "blogsearch", "articles")
	if err := mongo.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		_ = mongo.Close(ctx)
	}()

	pg := db.NewPostgresClient(db.PostgresConfig{DSN: postgresDSN})
	if err := pg.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer func() {
		_ = pg.Close()
	}()

	rep, err := replication.NewReplicator(replication.Config{
		Mongo:    mongo,
		Postgres: pg,
	})
	if err != nil {
		log.Fatalf("Failed to create replicator: %v", err)
	}

	if err := rep.ReplicateArticlesMongoToPostgres(ctx); err != nil {
		log.Fatalf("Replication failed: %v", err)
	}

	log.Println("Replication done!")
}

func runPaginatedFetch() {
	ctx := context.Background()

	// Get MongoDB connection string from environment or use default
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://admin:password@localhost:27017"
	}

	// Initialize database client
	dbClient := db.NewClient(mongoURI, "blogsearch", "articles")
	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	// Default configuration for se-radio.net
	baseURLPattern := "https://se-radio.net/page/%d"
	pagesPerBatch := 10
	urlFetcherWorkers := 3
	contentWorkers := 5

	// Parse command line arguments if provided
	// Usage: go run . paginate [baseURLPattern] [pagesPerBatch] [urlFetcherWorkers] [contentWorkers]
	if len(os.Args) >= 3 {
		baseURLPattern = os.Args[2]
	}
	if len(os.Args) >= 4 {
		if val, err := strconv.Atoi(os.Args[3]); err == nil {
			pagesPerBatch = val
		}
	}
	if len(os.Args) >= 5 {
		if val, err := strconv.Atoi(os.Args[4]); err == nil {
			urlFetcherWorkers = val
		}
	}
	if len(os.Args) >= 6 {
		if val, err := strconv.Atoi(os.Args[5]); err == nil {
			contentWorkers = val
		}
	}

	// Create TwoLevelManager
	manager := worker.NewTwoLevelManager(worker.Config{
		URLFetcherWorkers: urlFetcherWorkers,
		ContentWorkers:    contentWorkers,
		DBClient:          dbClient,
		PagesPerBatch:     pagesPerBatch,
		BaseURLPattern:    baseURLPattern,
		Extractor:         urls.ExtractSERadioURLs,
	})

	log.Printf("Starting paginated fetch with configuration:")
	log.Printf("  Base URL Pattern: %s", baseURLPattern)
	log.Printf("  Pages per batch: %d", pagesPerBatch)
	log.Printf("  URL Fetcher Workers: %d", urlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	log.Println("Processing pages (will continue until no more pages found)...")

	// Process pages - will continue until no more pages found
	if err := manager.ProcessPaginatedPages(ctx); err != nil {
		log.Fatalf("Failed to process paginated pages: %v", err)
	}

	// Get final count
	articles, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get article count: %v", err)
	} else {
		log.Printf("Successfully processed and saved %d articles to database", len(articles))
	}

	log.Println("All done!")
}
