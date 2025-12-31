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
	"blog-search/pkg/httpclient"
	"blog-search/pkg/pipeline"
	"blog-search/pkg/replication"
	"blog-search/pkg/sites"
	"blog-search/pkg/urls"
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
	//
	// Example with categories:
	//   go run . pipeline category https://site.com category1,category2,category3
	//
	// Example with HTML file:
	//   go run . pipeline htmlfile html-page-examples/se-radio-page.html se-radio
	if len(os.Args) > 1 && os.Args[1] == "pipeline" {
		runPipeline()
		return
	}

	// No subcommand provided, print usage
	log.Fatalf("Please provide a subcommand: replicate, extract, or pipeline")
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

	// Support Postgres (including Supabase, which is just PostgreSQL)
	var dbProvider db.DBProvider
	supabaseConnStr := os.Getenv("SUPABASE_CONNECTION_STRING")
	postgresDSN := os.Getenv("POSTGRES_DSN")

	// Use SUPABASE_CONNECTION_STRING if provided, otherwise use POSTGRES_DSN
	dsn := supabaseConnStr
	if dsn == "" {
		dsn = postgresDSN
	}

	if dsn == "" {
		log.Fatalf("Database connection required for replication.\n" +
			"Set either SUPABASE_CONNECTION_STRING or POSTGRES_DSN environment variable.")
	}

	pg := db.NewPostgresClient(db.PostgresConfig{DSN: dsn})
	if err := pg.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		_ = pg.Close()
	}()
	dbProvider = pg
	log.Println("Connected to database")

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
	case "category":
		p, baseURL = buildCategoryPipeline(dbClient, nonFlagArgs, filters)
	case "htmlfile":
		p, baseURL = buildHTMLFilePipeline(dbClient, nonFlagArgs, filters)
	default:
		log.Fatalf("Unknown pipeline type: %s. Use 'sitemap', 'rss', 'paginate', 'category', or 'htmlfile'", pipelineType)
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
		log.Fatalf("Usage: go run . pipeline [sitemap|rss|paginate|htmlfile] [URL/pattern/file] [additional args...] [-url-filter=<path>]")
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
	// numberOfPages defaults to 0 (unlimited, check existence) for backward compatibility
	numberOfPages := 0
	var p *pipeline.Pipeline
	if strings.Contains(baseURLArg, "dataengineeringpodcast.com") {
		p = pipeline.DataEngineeringPodcastPipelineBuilder(dbClient, baseURLArg, pagePattern, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages, extractor, filters...)
		log.Printf("Using DataEngineeringPodcastPipelineBuilder (with transcript extraction)")
	} else {
		p = pipeline.PaginationPipelineBuilder(dbClient, baseURLArg, pagePattern, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages, extractor, filters...)
	}
	logPaginationConfig(baseURLArg, pagePattern, args, extractor, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages, filters)

	return p, baseURLArg
}

// buildCategoryPipeline builds a category pipeline from command-line arguments
func buildCategoryPipeline(dbClient *db.Client, args []string, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	if len(args) < 3 {
		log.Fatalf("Usage: go run . pipeline category <base-url> <category1,category2,...> [extractor-type] [html-fetcher-workers] [content-workers] [-url-filter=<path>]")
	}

	baseURLArg := args[1]
	categoriesStr := args[2]
	extractor := determineExtractor(args, baseURLArg)
	htmlFetcherWorkers := parseWorkerCount(args, 4, 3)
	contentWorkers := parseWorkerCount(args, 5, 5)

	// Parse comma-separated categories
	categories := parseCategories(categoriesStr)

	var p *pipeline.Pipeline
	p = pipeline.CategoryPipelineBuilder(dbClient, baseURLArg, categories, htmlFetcherWorkers, contentWorkers, extractor, filters...)
	logCategoryConfig(baseURLArg, categories, args, extractor, htmlFetcherWorkers, contentWorkers, filters)

	return p, baseURLArg
}

// parseCategories parses comma-separated categories and trims whitespace
func parseCategories(categoriesStr string) []string {
	parts := strings.Split(categoriesStr, ",")
	categories := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			categories = append(categories, trimmed)
		}
	}
	return categories
}

// logCategoryConfig logs the category pipeline configuration
func logCategoryConfig(baseURL string, categories []string, args []string, extractor urls.URLExtractor, htmlFetcherWorkers, contentWorkers int, filters []urls.UrlFilter) {
	log.Printf("Running Category pipeline for %s with categories: %v", baseURL, categories)

	extractorName := "se-radio (default)"
	if len(args) >= 4 {
		extractorName = args[3]
	} else if strings.Contains(baseURL, "dataengineeringpodcast.com") {
		extractorName = "data-engineering-podcast (auto-detected)"
	}

	log.Printf("  Extractor: %s", extractorName)
	log.Printf("  HTML Fetcher Workers: %d", htmlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	if len(filters) > 0 {
		log.Printf("  Applied %d URL filter(s)", len(filters))
	}
}

// buildHTMLFilePipeline builds an HTML file pipeline from command-line arguments
func buildHTMLFilePipeline(dbClient *db.Client, args []string, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	if len(args) < 3 {
		log.Fatalf("Usage: go run . pipeline htmlfile <html-file-path> <extractor-type> [url-fetcher-workers] [content-workers] [client-type] [-url-filter=<path>]\n" +
			"  extractor-type: se-radio, data-engineering-podcast, generic\n" +
			"  client-type: browser (for sites that block curl) or cloudflare (default)")
	}

	htmlFilePath := args[1]
	extractorType := args[2]
	urlFetcherWorkers := parseWorkerCount(args, 3, 1)
	contentWorkers := parseWorkerCount(args, 4, 5)
	clientType := parseClientType(args, 5, httpclient.CloudflareClient)

	extractor := getExtractorByType(extractorType)
	if extractor == nil {
		log.Fatalf("Unknown extractor type: %s. Available: se-radio, data-engineering-podcast, generic", extractorType)
	}

	p := pipeline.HTMLFilePipelineBuilderWithClient(dbClient, urlFetcherWorkers, contentWorkers, extractor, clientType, filters...)
	logHTMLFileConfig(htmlFilePath, extractorType, urlFetcherWorkers, contentWorkers, clientType, filters)

	return p, htmlFilePath
}

// parseClientType parses a client type from args at the given index, with a default value
func parseClientType(args []string, index int, defaultValue httpclient.ClientType) httpclient.ClientType {
	if len(args) > index {
		clientTypeStr := strings.ToLower(args[index])
		switch clientTypeStr {
		case "browser":
			return httpclient.BrowserClient
		case "cloudflare":
			return httpclient.CloudflareClient
		default:
			log.Printf("Unknown client type '%s', using default (%s)", clientTypeStr, defaultValue)
		}
	}
	return defaultValue
}

// logHTMLFileConfig logs the HTML file pipeline configuration
func logHTMLFileConfig(htmlFilePath, extractorType string, urlFetcherWorkers, contentWorkers int, clientType httpclient.ClientType, filters []urls.UrlFilter) {
	log.Printf("Running HTML File pipeline for file: %s", htmlFilePath)
	log.Printf("  Extractor: %s", extractorType)
	log.Printf("  URL Fetcher Workers: %d", urlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	log.Printf("  HTTP Client Type: %s", clientType)
	if len(filters) > 0 {
		log.Printf("  Applied %d URL filter(s)", len(filters))
	}
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
func logPaginationConfig(baseURL, pagePattern string, args []string, extractor urls.URLExtractor, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages int, filters []urls.UrlFilter) {
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
	if numberOfPages > 0 {
		log.Printf("  Number of pages: %d (fixed, existence checks bypassed)", numberOfPages)
	} else {
		log.Printf("  Number of pages: unlimited (will check existence for each page)")
	}
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
