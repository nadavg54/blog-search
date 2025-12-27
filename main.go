package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"blog-search/pkg/db"
	"blog-search/pkg/pipeline"
	"blog-search/pkg/replication"
	"blog-search/pkg/sites"
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

	// Subcommand: extract (test URL extraction from HTML file)
	//
	// Example:
	//   go run . extract html-page-examples/se-radio-page.html se-radio
	//   go run . extract html-page-examples/data-engineering-podcast-page.html data-engineering-podcast
	if len(os.Args) > 1 && os.Args[1] == "extract" {
		runExtract()
		return
	}

	// Subcommand: pipeline (use new pipeline system)
	//
	// Example with sitemap:
	//   MONGO_URI="mongodb://admin:password@localhost:27017" \
	//   go run . pipeline sitemap https://engineering.fb.com/post-sitemap.xml
	//
	// Example with RSS:
	//   go run . pipeline rss https://example.com/feed.xml
	//
	// Example with pagination:
	//   go run . pipeline paginate https://se-radio.net /page/%d
	if len(os.Args) > 1 && os.Args[1] == "pipeline" {
		runPipeline()
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

	mongo := db.NewClient(mongoURI, "blogsearch", "articles")
	if err := mongo.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		_ = mongo.Close(ctx)
	}()

	// Support both Postgres and Supabase
	var dbProvider db.DBProvider
	supabaseConnStr := os.Getenv("SUPABASE_CONNECTION_STRING")
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")
	supabasePassword := os.Getenv("SUPABASE_PASSWORD")
	postgresDSN := os.Getenv("POSTGRES_DSN")

	// Check if Supabase is configured
	hasSupabaseConfig := supabaseConnStr != "" || (supabaseURL != "" && supabasePassword != "")
	hasSupabaseURLKey := supabaseURL != "" && supabaseKey != ""

	if hasSupabaseConfig || hasSupabaseURLKey {
		// Use Supabase client (will work in REST API mode if only URL+key provided)
		supabaseClient := db.NewSupabaseClient(db.SupabaseConfig{
			ConnectionString: supabaseConnStr,
			SupabaseURL:      supabaseURL,
			SupabaseKey:      supabaseKey,
			Password:         supabasePassword,
		})
		if err := supabaseClient.Connect(ctx); err != nil {
			log.Fatalf("Failed to connect to Supabase: %v", err)
		}
		defer func() {
			_ = supabaseClient.Close()
		}()

		// Check if we have direct DB access (required for replication)
		if !supabaseClient.HasDirectDB() {
			log.Fatalf("Direct database connection is required for replication.\n" +
				"You provided Supabase URL and key, but replication needs direct SQL access.\n" +
				"Please set SUPABASE_PASSWORD (your database password from Supabase dashboard) or SUPABASE_CONNECTION_STRING.\n" +
				"Note: The API key is for REST API calls, not for direct Postgres connections.")
		}

		dbProvider = supabaseClient
		log.Println("Using Supabase client")
	} else if postgresDSN != "" {
		// Use standard Postgres client
		pg := db.NewPostgresClient(db.PostgresConfig{DSN: postgresDSN})
		if err := pg.Connect(ctx); err != nil {
			log.Fatalf("Failed to connect to Postgres: %v", err)
		}
		defer func() {
			_ = pg.Close()
		}()
		dbProvider = pg
		log.Println("Using Postgres client")
	} else {
		log.Fatalf("Database connection required for replication.\n" +
			"Options:\n" +
			"  1. SUPABASE_CONNECTION_STRING (full connection string)\n" +
			"  2. SUPABASE_URL + SUPABASE_PASSWORD (we'll build the connection string)\n" +
			"  3. POSTGRES_DSN (standard Postgres connection string)")
	}

	rep, err := replication.NewReplicator(replication.Config{
		Mongo:    mongo,
		Postgres: dbProvider,
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
		Extractor:         sites.ExtractSERadioURLs,
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

func runPipeline() {
	ctx := context.Background()

	dbClient := initializeDatabase(ctx)
	defer dbClient.Close(ctx)

	urlFilterPath, nonFlagArgs := parsePipelineFlags()
	filters := buildURLFilters(urlFilterPath)
	pipelineType := nonFlagArgs[0]

	var p *pipeline.Pipeline
	var baseURL string

	switch pipelineType {
	case "sitemap":
		p, baseURL = buildSitemapPipeline(dbClient, nonFlagArgs, filters)
	case "rss":
		p, baseURL = buildRSSPipeline(dbClient, nonFlagArgs, filters)
	case "paginate":
		p, baseURL = buildPaginationPipeline(dbClient, nonFlagArgs, filters)
	default:
		log.Fatalf("Unknown pipeline type: %s. Use 'sitemap', 'rss', or 'paginate'", pipelineType)
	}

	runPipelineAndReport(ctx, p, baseURL, dbClient)
}

// initializeDatabase connects to MongoDB and returns the client
func initializeDatabase(ctx context.Context) *db.Client {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://admin:password@localhost:27017"
	}

	dbClient := db.NewClient(mongoURI, "blogsearch", "articles")
	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	return dbClient
}

// parsePipelineFlags parses command-line flags and separates flag args from non-flag args
func parsePipelineFlags() (*string, []string) {
	if len(os.Args) < 3 {
		log.Fatalf("Usage: go run . pipeline [sitemap|rss|paginate] [URL/pattern] [additional args...] [-url-filter=<path>]")
	}

	fs := flag.NewFlagSet("pipeline", flag.ExitOnError)
	urlFilterPath := fs.String("url-filter", "", "Filter URLs to only include those containing this path segment (e.g., '/blog')")

	args := os.Args[2:]
	var nonFlagArgs []string
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if err := fs.Parse(args[i:]); err != nil {
				log.Fatalf("Failed to parse flags: %v", err)
			}
			nonFlagArgs = args[:i]
			break
		}
		nonFlagArgs = append(nonFlagArgs, arg)
	}

	if len(nonFlagArgs) == len(args) {
		fs.Parse([]string{})
	}

	return urlFilterPath, nonFlagArgs
}

// buildURLFilters creates URL filters from the filter path flag
func buildURLFilters(urlFilterPath *string) []urls.UrlFilter {
	var filters []urls.UrlFilter
	if *urlFilterPath != "" {
		log.Printf("Adding URL filter: must contain path '%s'", *urlFilterPath)
		filters = append(filters, urls.NewContainsPathFilter(*urlFilterPath))
	}
	return filters
}

// buildSitemapPipeline builds a sitemap pipeline from command-line arguments
func buildSitemapPipeline(dbClient *db.Client, args []string, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	if len(args) < 2 {
		log.Fatalf("Usage: go run . pipeline sitemap <sitemap-url> [url-fetcher-workers] [content-workers] [-url-filter=<path>]")
	}

	sitemapURL := args[1]
	urlFetcherWorkers := parseWorkerCount(args, 2, 2)
	contentWorkers := parseWorkerCount(args, 3, 3)

	p := pipeline.SitemapPipelineBuilder(dbClient, urlFetcherWorkers, contentWorkers, filters...)
	logPipelineConfig("sitemap", urlFetcherWorkers, contentWorkers, filters)

	return p, sitemapURL
}

// buildRSSPipeline builds an RSS pipeline from command-line arguments
func buildRSSPipeline(dbClient *db.Client, args []string, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	if len(args) < 2 {
		log.Fatalf("Usage: go run . pipeline rss <rss-url> [url-fetcher-workers] [content-workers] [-url-filter=<path>]")
	}

	rssURL := args[1]
	urlFetcherWorkers := parseWorkerCount(args, 2, 2)
	contentWorkers := parseWorkerCount(args, 3, 3)

	p := pipeline.RSSPipelineBuilder(dbClient, urlFetcherWorkers, contentWorkers, filters...)
	logPipelineConfig("RSS", urlFetcherWorkers, contentWorkers, filters)

	return p, rssURL
}

// buildPaginationPipeline builds a pagination pipeline from command-line arguments
func buildPaginationPipeline(dbClient *db.Client, args []string, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	if len(args) < 3 {
		log.Fatalf("Usage: go run . pipeline paginate <base-url> <page-pattern> [extractor-type] [pages-per-batch] [page-gen-workers] [html-fetcher-workers] [content-workers] [-url-filter=<path>]")
	}

	baseURLArg := args[1]
	pagePattern := args[2]
	extractor := determineExtractor(args, baseURLArg)
	pagesPerBatch := parseWorkerCount(args, 4, 10)
	pageGenWorkers := parseWorkerCount(args, 5, 1)
	htmlFetcherWorkers := parseWorkerCount(args, 6, 3)
	contentWorkers := parseWorkerCount(args, 7, 5)

	// Use DataEngineeringPodcastPipelineBuilder if URL is for dataengineeringpodcast.com
	// This ensures transcript extraction is used instead of general content extraction
	var p *pipeline.Pipeline
	if strings.Contains(baseURLArg, "dataengineeringpodcast.com") {
		p = pipeline.DataEngineeringPodcastPipelineBuilder(dbClient, baseURLArg, pagePattern, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, extractor, filters...)
		log.Printf("Using DataEngineeringPodcastPipelineBuilder (with transcript extraction)")
	} else {
		p = pipeline.PaginationPipelineBuilder(dbClient, baseURLArg, pagePattern, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, extractor, filters...)
	}
	logPaginationConfig(baseURLArg, pagePattern, args, extractor, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, filters)

	return p, baseURLArg
}

// parseWorkerCount parses a worker count from args at the given index, with a default value
func parseWorkerCount(args []string, index int, defaultValue int) int {
	if len(args) > index {
		if val, err := strconv.Atoi(args[index]); err == nil {
			return val
		}
	}
	return defaultValue
}

// determineExtractor determines the URL extractor based on args or URL auto-detection
func determineExtractor(args []string, baseURL string) urls.URLExtractor {
	if len(args) >= 4 {
		extractorType := args[3]
		switch extractorType {
		case "se-radio":
			return sites.ExtractSERadioURLs
		case "data-engineering-podcast":
			return sites.ExtractDataEngineeringPodcastURLs
		case "generic":
			return sites.ExtractGenericURLs
		default:
			log.Printf("Unknown extractor type '%s', using default (se-radio)", extractorType)
		}
	}

	if strings.Contains(baseURL, "dataengineeringpodcast.com") {
		return sites.ExtractDataEngineeringPodcastURLs
	}

	return sites.ExtractSERadioURLs
}

// logPipelineConfig logs the pipeline configuration
func logPipelineConfig(pipelineType string, urlFetcherWorkers, contentWorkers int, filters []urls.UrlFilter) {
	log.Printf("Running %s pipeline with %d URL fetcher workers, %d content workers", pipelineType, urlFetcherWorkers, contentWorkers)
	if len(filters) > 0 {
		log.Printf("Applied %d URL filter(s)", len(filters))
	}
}

// logPaginationConfig logs the pagination pipeline configuration
func logPaginationConfig(baseURL, pagePattern string, args []string, extractor urls.URLExtractor, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers int, filters []urls.UrlFilter) {
	log.Printf("Running Pagination pipeline for %s with pattern %s:", baseURL, pagePattern)

	extractorName := "se-radio (default)"
	if len(args) >= 4 {
		extractorName = args[3]
	} else if strings.Contains(baseURL, "dataengineeringpodcast.com") {
		extractorName = "data-engineering-podcast (auto-detected)"
	}

	log.Printf("  Extractor: %s", extractorName)
	log.Printf("  Pages per batch: %d", pagesPerBatch)
	log.Printf("  Page Generator Workers: %d", pageGenWorkers)
	log.Printf("  HTML Fetcher Workers: %d", htmlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	if len(filters) > 0 {
		log.Printf("  Applied %d URL filter(s)", len(filters))
	}
}

// runPipelineAndReport runs the pipeline and reports the results
func runPipelineAndReport(ctx context.Context, p *pipeline.Pipeline, baseURL string, dbClient *db.Client) {
	log.Printf("Starting pipeline with base URL: %s", baseURL)
	if err := p.Run(ctx, baseURL); err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}

	articles, err := dbClient.GetAllArticles(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get article count: %v", err)
	} else {
		log.Printf("Successfully processed and saved %d articles to database", len(articles))
	}

	log.Println("Pipeline completed!")
}

// runExtract extracts URLs from an HTML file using a specified extractor
func runExtract() {
	if len(os.Args) < 4 {
		log.Fatalf("Usage: go run . extract <html-file-path> <extractor-type>\n" +
			"  extractor-type: se-radio, data-engineering-podcast, generic")
	}

	htmlFilePath := os.Args[2]
	extractorType := os.Args[3]

	// Read HTML file
	htmlContent, err := os.ReadFile(htmlFilePath)
	if err != nil {
		log.Fatalf("Failed to read HTML file %s: %v", htmlFilePath, err)
	}

	// Get the appropriate extractor
	extractor := getExtractorByType(extractorType)
	if extractor == nil {
		log.Fatalf("Unknown extractor type: %s. Available: se-radio, data-engineering-podcast", extractorType)
	}

	// Extract URLs
	urls, err := extractor(string(htmlContent))
	if err != nil {
		log.Fatalf("Failed to extract URLs: %v", err)
	}

	// Print results
	fmt.Printf("\n=== Extracted %d URLs from %s ===\n\n", len(urls), htmlFilePath)
	for i, url := range urls {
		fmt.Printf("%d. Title: %s\n", i+1, url.Title)
		fmt.Printf("   URL: %s\n\n", url.Location)
	}
}

// getExtractorByType returns the appropriate extractor function based on type
func getExtractorByType(extractorType string) urls.URLExtractor {
	switch extractorType {
	case "se-radio":
		return sites.ExtractSERadioURLs
	case "data-engineering-podcast":
		return sites.ExtractDataEngineeringPodcastURLs
	case "generic":
		return sites.ExtractGenericURLs
	default:
		return nil
	}
}
