package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"blog-search/pkg/db"
	"blog-search/pkg/httpclient"
	"blog-search/pkg/pipeline"
	"blog-search/pkg/replication"
	"blog-search/pkg/sites"
	"blog-search/pkg/urls"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := ParseArgs(os.Args[2:])

	switch command {
	case "replicate":
		runReplication()
	case "extract":
		runExtract(args)
	case "pipeline":
		runPipeline(args)
	default:
		log.Fatalf("Unknown command: %s. Use 'replicate', 'extract', or 'pipeline'", command)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  go run ./cmd/refactored replicate")
	fmt.Println("  go run ./cmd/refactored extract -file=<path> -extractor=<type>")
	fmt.Println("  go run ./cmd/refactored pipeline <type> [flags]")
	fmt.Println("")
	fmt.Println("Pipeline types: sitemap, rss, paginate, category, htmlfile, directpage")
}

// ============================================================================
// REPLICATE COMMAND
// ============================================================================

func runReplication() {
	ctx := context.Background()

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

	var dbProvider db.DBProvider
	localPostgresDSN := os.Getenv("LOCAL_POSTGRES_DSN")
	supabaseConnStr := os.Getenv("SUPABASE_CONNECTION_STRING")
	postgresDSN := os.Getenv("POSTGRES_DSN")

	dsn := localPostgresDSN
	if dsn == "" {
		dsn = supabaseConnStr
	}
	if dsn == "" {
		dsn = postgresDSN
	}

	if dsn == "" {
		log.Fatalf("Database connection required for replication.\n" +
			"Set one of: LOCAL_POSTGRES_DSN, SUPABASE_CONNECTION_STRING, or POSTGRES_DSN environment variable.")
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

// ============================================================================
// EXTRACT COMMAND
// ============================================================================

func runExtract(args Args) {
	usage := "Usage: go run ./cmd/refactored extract -file=<path> -extractor=<type>"

	file := args.RequireString("file", usage)
	extractorType := args.RequireString("extractor", usage)

	htmlContent, err := os.ReadFile(file)
	if err != nil {
		log.Fatalf("Failed to read HTML file %s: %v", file, err)
	}

	extractor := getExtractorByType(extractorType)
	if extractor == nil {
		log.Fatalf("Unknown extractor type: %s. Available: se-radio, data-engineering-podcast, generic", extractorType)
	}

	urls, err := extractor(string(htmlContent))
	if err != nil {
		log.Fatalf("Failed to extract URLs: %v", err)
	}

	fmt.Printf("\n=== Extracted %d URLs from %s ===\n\n", len(urls), file)
	for i, url := range urls {
		fmt.Printf("%d. Title: %s\n", i+1, url.Title)
		fmt.Printf("   URL: %s\n\n", url.Location)
	}
}

// ============================================================================
// PIPELINE COMMAND
// ============================================================================

func runPipeline(args Args) {
	pipelineType := args.GetPositional(0)
	if pipelineType == "" {
		log.Fatalf("Usage: go run ./cmd/refactored pipeline <type> [flags]\n" +
			"Types: sitemap, rss, paginate, category, htmlfile, directpage")
	}

	ctx := context.Background()
	dbClient := initializeDatabase(ctx)
	defer dbClient.Close(ctx)

	filters := buildURLFilters(args)

	var p *pipeline.Pipeline
	var baseURL string

	switch pipelineType {
	case "sitemap":
		p, baseURL = buildSitemapPipeline(dbClient, args, filters)
	case "rss":
		p, baseURL = buildRSSPipeline(dbClient, args, filters)
	case "paginate":
		p, baseURL = buildPaginationPipeline(dbClient, args, filters)
	case "category":
		p, baseURL = buildCategoryPipeline(dbClient, args, filters)
	case "htmlfile":
		p, baseURL = buildHTMLFilePipeline(dbClient, args, filters)
	case "directpage":
		p, baseURL = buildDirectPagePipeline(dbClient, args)
	default:
		log.Fatalf("Unknown pipeline type: %s. Use 'sitemap', 'rss', 'paginate', 'category', 'htmlfile', or 'directpage'", pipelineType)
	}

	runPipelineAndReport(ctx, p, baseURL, dbClient)
}

func buildURLFilters(args Args) []urls.UrlFilter {
	var filters []urls.UrlFilter
	urlFilterPath := args.GetString("url-filter", "")
	if urlFilterPath != "" {
		log.Printf("Adding URL filter: must contain path '%s'", urlFilterPath)
		filters = append(filters, urls.NewContainsPathFilter(urlFilterPath))
	}
	return filters
}

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

// ============================================================================
// SITEMAP PIPELINE
// ============================================================================

func buildSitemapPipeline(dbClient *db.Client, args Args, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline sitemap -url=<sitemap-url> [-url-fetcher-workers=<n>] [-content-workers=<n>] [-url-filter=<path>]"

	url := args.RequireString("url", usage)
	urlFetcherWorkers := args.GetInt("url-fetcher-workers", 2)
	contentWorkers := args.GetInt("content-workers", 3)

	p := pipeline.SitemapPipelineBuilder(dbClient, urlFetcherWorkers, contentWorkers, filters...)
	logPipelineConfig("sitemap", urlFetcherWorkers, contentWorkers, filters)

	return p, url
}

// ============================================================================
// RSS PIPELINE
// ============================================================================

func buildRSSPipeline(dbClient *db.Client, args Args, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline rss -url=<rss-url> [-url-fetcher-workers=<n>] [-content-workers=<n>] [-url-filter=<path>]"

	url := args.RequireString("url", usage)
	urlFetcherWorkers := args.GetInt("url-fetcher-workers", 2)
	contentWorkers := args.GetInt("content-workers", 3)

	p := pipeline.RSSPipelineBuilder(dbClient, urlFetcherWorkers, contentWorkers, filters...)
	logPipelineConfig("RSS", urlFetcherWorkers, contentWorkers, filters)

	return p, url
}

// ============================================================================
// PAGINATION PIPELINE
// ============================================================================

func buildPaginationPipeline(dbClient *db.Client, args Args, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline paginate -url=<base-url> -pattern=<page-pattern> [-extractor=<type>] [-pages-per-batch=<n>] [-page-gen-workers=<n>] [-url-fetcher-workers=<n>] [-content-workers=<n>] [-number-of-pages=<n>] [-url-filter=<path>]"

	baseURL := args.RequireString("url", usage)
	pagePattern := args.RequireString("pattern", usage)
	extractorType := args.GetString("extractor", "")
	pagesPerBatch := args.GetInt("pages-per-batch", 10)
	pageGenWorkers := args.GetInt("page-gen-workers", 1)
	urlFetcherWorkers := args.GetInt("url-fetcher-workers", 3)
	contentWorkers := args.GetInt("content-workers", 5)
	numberOfPages := args.GetInt("number-of-pages", 0)

	extractor := determineExtractor(extractorType, baseURL)

	var p *pipeline.Pipeline
	if strings.Contains(baseURL, "dataengineeringpodcast.com") {
		p = pipeline.DataEngineeringPodcastPipelineBuilder(dbClient, baseURL, pagePattern, pagesPerBatch, pageGenWorkers, urlFetcherWorkers, contentWorkers, numberOfPages, extractor, filters...)
		log.Printf("Using DataEngineeringPodcastPipelineBuilder (with transcript extraction)")
	} else {
		p = pipeline.PaginationPipelineBuilder(dbClient, baseURL, pagePattern, pagesPerBatch, pageGenWorkers, urlFetcherWorkers, contentWorkers, numberOfPages, extractor, filters...)
	}

	logPaginationConfig(baseURL, pagePattern, extractorType, pagesPerBatch, pageGenWorkers, urlFetcherWorkers, contentWorkers, numberOfPages, filters)

	return p, baseURL
}

// ============================================================================
// CATEGORY PIPELINE
// ============================================================================

func buildCategoryPipeline(dbClient *db.Client, args Args, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline category -url=<base-url> -categories=<cat1,cat2,...> [-extractor=<type>] [-url-fetcher-workers=<n>] [-content-workers=<n>] [-url-filter=<path>]"

	baseURL := args.RequireString("url", usage)
	categoriesStr := args.RequireString("categories", usage)
	extractorType := args.GetString("extractor", "")
	urlFetcherWorkers := args.GetInt("url-fetcher-workers", 3)
	contentWorkers := args.GetInt("content-workers", 5)

	categories := parseCategories(categoriesStr)
	extractor := determineExtractor(extractorType, baseURL)

	p := pipeline.CategoryPipelineBuilder(dbClient, baseURL, categories, urlFetcherWorkers, contentWorkers, extractor, filters...)
	logCategoryConfig(baseURL, categories, extractorType, urlFetcherWorkers, contentWorkers, filters)

	return p, baseURL
}

// ============================================================================
// HTML FILE PIPELINE
// ============================================================================

func buildHTMLFilePipeline(dbClient *db.Client, args Args, filters []urls.UrlFilter) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline htmlfile -file=<html-file-path> -extractor=<type> [-base-url=<base-url>] [-url-fetcher-workers=<n>] [-content-workers=<n>] [-client-type=<browser|cloudflare>] [-url-filter=<path>]"

	htmlFile := args.RequireString("file", usage)
	extractorType := args.RequireString("extractor", usage)
	baseURL := args.GetString("base-url", "")
	urlFetcherWorkers := args.GetInt("url-fetcher-workers", 1)
	contentWorkers := args.GetInt("content-workers", 5)
	clientTypeStr := args.GetString("client-type", "cloudflare")

	extractor := getExtractorByType(extractorType)
	if extractor == nil {
		log.Fatalf("Unknown extractor type: %s. Available: se-radio, data-engineering-podcast, generic", extractorType)
	}

	clientType := parseClientType(clientTypeStr, httpclient.CloudflareClient)

	var p *pipeline.Pipeline
	if baseURL != "" {
		p = pipeline.HTMLFilePipelineBuilderWithClientAndBaseURL(dbClient, urlFetcherWorkers, contentWorkers, extractor, clientType, baseURL, filters...)
		logHTMLFileConfig(htmlFile, extractorType, urlFetcherWorkers, contentWorkers, clientType, filters, baseURL)
	} else {
		p = pipeline.HTMLFilePipelineBuilderWithClient(dbClient, urlFetcherWorkers, contentWorkers, extractor, clientType, filters...)
		logHTMLFileConfig(htmlFile, extractorType, urlFetcherWorkers, contentWorkers, clientType, filters, "")
	}

	return p, htmlFile
}

// ============================================================================
// DIRECT PAGE PIPELINE
// ============================================================================

func buildDirectPagePipeline(dbClient *db.Client, args Args) (*pipeline.Pipeline, string) {
	usage := "Usage: go run ./cmd/refactored pipeline directpage -url=<base-url> -pattern=<page-pattern> [-extractor=<type>] [-pages-per-batch=<n>] [-page-gen-workers=<n>] [-content-workers=<n>] [-number-of-pages=<n>]"

	baseURL := args.RequireString("url", usage)
	pagePattern := args.RequireString("pattern", usage)
	extractorType := args.GetString("extractor", "")
	pagesPerBatch := args.GetInt("pages-per-batch", 10)
	pageGenWorkers := args.GetInt("page-gen-workers", 1)
	contentWorkers := args.GetInt("content-workers", 5)
	numberOfPages := args.GetInt("number-of-pages", 0)

	extractor := determineExtractor(extractorType, baseURL)

	p := pipeline.DirectPagePipelineBuilder(dbClient, baseURL, pagePattern, pagesPerBatch, pageGenWorkers, contentWorkers, numberOfPages, extractor)
	logDirectPageConfig(baseURL, pagePattern, extractorType, pagesPerBatch, pageGenWorkers, contentWorkers, numberOfPages)

	return p, baseURL
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

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

func determineExtractor(extractorType string, baseURL string) urls.URLExtractor {
	if extractorType != "" {
		extractor := getExtractorByType(extractorType)
		if extractor != nil {
			return extractor
		}
		log.Printf("Unknown extractor type '%s', using auto-detection", extractorType)
	}

	return sites.ExtractGenericURLs
}

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

func parseClientType(clientTypeStr string, defaultValue httpclient.ClientType) httpclient.ClientType {
	clientTypeStr = strings.ToLower(clientTypeStr)
	switch clientTypeStr {
	case "browser":
		return httpclient.BrowserClient
	case "cloudflare":
		return httpclient.CloudflareClient
	default:
		log.Printf("Unknown client type '%s', using default (%s)", clientTypeStr, defaultValue)
		return defaultValue
	}
}

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
