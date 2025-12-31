package pipeline

import (
	"blog-search/pkg/content"
	"blog-search/pkg/db"
	"blog-search/pkg/httpclient"
	"blog-search/pkg/urls"
)

// RSSPipelineBuilder builds a pipeline for RSS feeds
// Pipeline: BaseURL → [RSS Fetcher] → [Content Consumer]
func RSSPipelineBuilder(dbClient *db.Client, urlFetcherWorkers, contentWorkers int, filters ...urls.UrlFilter) *Pipeline {
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewBasicURLFetcherWithFilters(urls.NewRSSParser(), filters)
	} else {
		fetcher = NewBasicURLFetcher(urls.NewRSSParser())
	}

	step := PipelineStep{
		Name:        "RSS Fetcher",
		WorkerCount: urlFetcherWorkers,
		Generator:   nil, // Uses Fetcher with baseURL
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessor(),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step}, consumer)
}

// SitemapPipelineBuilder builds a pipeline for Sitemaps
// Pipeline: BaseURL → [Sitemap Fetcher] → [Content Consumer]
func SitemapPipelineBuilder(dbClient *db.Client, urlFetcherWorkers, contentWorkers int, filters ...urls.UrlFilter) *Pipeline {
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewBasicURLFetcherWithFilters(urls.NewSitemapParser(), filters)
	} else {
		fetcher = NewBasicURLFetcher(urls.NewSitemapParser())
	}

	step := PipelineStep{
		Name:        "Sitemap Fetcher",
		WorkerCount: urlFetcherWorkers,
		Generator:   nil, // Uses Fetcher with baseURL
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessor(),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step}, consumer)
}

// PaginationPipelineBuilder builds a pipeline for paginated HTML sites
// Pipeline: [Page Range Generator] → [HTML Page Fetcher] → [Content Consumer]
// baseURL: the base URL (e.g., "https://site.com")
// pagePattern: the pattern for page URLs with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
// numberOfPages: number of pages to generate (0 = unlimited, check existence for each page)
func PaginationPipelineBuilder(dbClient *db.Client, baseURL, pagePattern string, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages int, extractor urls.URLExtractor, filters ...urls.UrlFilter) *Pipeline {
	// Step 1: Generate page URLs (uses Generator, not Fetcher)
	step1 := PipelineStep{
		Name:        "Page Range Generator",
		WorkerCount: pageGenWorkers,
		Generator:   NewPageRangeGenerator(baseURL, pagePattern, pagesPerBatch, numberOfPages, extractor),
		Fetcher:     nil, // First step uses Generator
	}

	// Step 2: Extract article URLs from each page (uses Fetcher with filters)
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewHTMLPageFetcherWithFilters(extractor, filters)
	} else {
		fetcher = NewHTMLPageFetcher(extractor)
	}

	step2 := PipelineStep{
		Name:        "HTML Page Fetcher",
		WorkerCount: htmlFetcherWorkers,
		Generator:   nil, // Subsequent steps use Fetcher
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessor(),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step1, step2}, consumer)
}

// DataEngineeringPodcastPipelineBuilder builds a pipeline specifically for dataengineeringpodcast.com
// It uses the DataEngineeringPodcastExtractor which extracts transcript text instead of general content
// Pipeline: [Page Range Generator] → [HTML Page Fetcher] → [Content Consumer with Custom Extractor]
// baseURL: the base URL (e.g., "https://www.dataengineeringpodcast.com")
// pagePattern: the pattern for page URLs with %d placeholder (e.g., "/page/%d")
// numberOfPages: number of pages to generate (0 = unlimited, check existence for each page)
func DataEngineeringPodcastPipelineBuilder(dbClient *db.Client, baseURL, pagePattern string, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers, numberOfPages int, extractor urls.URLExtractor, filters ...urls.UrlFilter) *Pipeline {
	// Step 1: Generate page URLs (uses Generator, not Fetcher)
	step1 := PipelineStep{
		Name:        "Page Range Generator",
		WorkerCount: pageGenWorkers,
		Generator:   NewPageRangeGenerator(baseURL, pagePattern, pagesPerBatch, numberOfPages, extractor),
		Fetcher:     nil, // First step uses Generator
	}

	// Step 2: Extract article URLs from each page (uses Fetcher with filters)
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewHTMLPageFetcherWithFilters(extractor, filters)
	} else {
		fetcher = NewHTMLPageFetcher(extractor)
	}

	step2 := PipelineStep{
		Name:        "HTML Page Fetcher",
		WorkerCount: htmlFetcherWorkers,
		Generator:   nil, // Subsequent steps use Fetcher
		Fetcher:     fetcher,
	}

	// Use custom extractor for dataengineeringpodcast that extracts transcript
	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessorWithExtractor(content.NewDataEngineeringPodcastExtractor()),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step1, step2}, consumer)
}

// HTMLFilePipelineBuilder builds a pipeline for extracting URLs from a local HTML file
// Pipeline: HTML File Path → [HTML File Fetcher] → [Content Consumer]
// Pass the HTML file path as the baseURL parameter when calling Run()
// extractor: function to extract URLs from the HTML content
// clientType: optional HTTP client type (defaults to CloudflareClient if not provided)
func HTMLFilePipelineBuilder(dbClient *db.Client, urlFetcherWorkers, contentWorkers int, extractor urls.URLExtractor, filters ...urls.UrlFilter) *Pipeline {
	return HTMLFilePipelineBuilderWithClient(dbClient, urlFetcherWorkers, contentWorkers, extractor, httpclient.CloudflareClient, filters...)
}

// HTMLFilePipelineBuilderWithClient builds a pipeline for extracting URLs from a local HTML file with a specific client type
// Pipeline: HTML File Path → [HTML File Fetcher] → [Content Consumer]
// Pass the HTML file path as the baseURL parameter when calling Run()
// extractor: function to extract URLs from the HTML content
// clientType: HTTP client type to use for fetching content (BrowserClient or CloudflareClient)
func HTMLFilePipelineBuilderWithClient(dbClient *db.Client, urlFetcherWorkers, contentWorkers int, extractor urls.URLExtractor, clientType httpclient.ClientType, filters ...urls.UrlFilter) *Pipeline {
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewHTMLPageFetcherWithFilters(extractor, filters)
	} else {
		fetcher = NewHTMLPageFetcher(extractor)
	}

	step := PipelineStep{
		Name:        "HTML File Fetcher",
		WorkerCount: urlFetcherWorkers,
		Generator:   nil, // Uses Fetcher with file path (passed as baseURL to Run())
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessorWithClient(clientType),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step}, consumer)
}

// CategoryPipelineBuilder builds a pipeline for category-based sites
// Pipeline: [Category Generator] → [HTML Page Fetcher] → [Content Consumer]
// baseURL: the base URL (e.g., "https://site.com")
// categories: list of categories to append to base URL (e.g., ["category1", "category2"])
func CategoryPipelineBuilder(dbClient *db.Client, baseURL string, categories []string, htmlFetcherWorkers, contentWorkers int, extractor urls.URLExtractor, filters ...urls.UrlFilter) *Pipeline {
	// Step 1: Generate category URLs (uses Generator, not Fetcher)
	step1 := PipelineStep{
		Name:        "Category Generator",
		WorkerCount: 1, // Generator runs once, worker count not used
		Generator:   NewPageCategoryGenerator(baseURL, categories),
		Fetcher:     nil, // First step uses Generator
	}

	// Step 2: Extract article URLs from each category page (uses Fetcher with filters)
	var fetcher URLFetcher
	if len(filters) > 0 {
		fetcher = NewHTMLPageFetcherWithFilters(extractor, filters)
	} else {
		fetcher = NewHTMLPageFetcher(extractor)
	}

	step2 := PipelineStep{
		Name:        "HTML Page Fetcher",
		WorkerCount: htmlFetcherWorkers,
		Generator:   nil, // Subsequent steps use Fetcher
		Fetcher:     fetcher,
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessor(),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step1, step2}, consumer)
}

// DirectPagePipelineBuilder builds a pipeline that generates page URLs and processes them directly as articles
// Pipeline: [Page Range Generator] → [Content Consumer]
// This is useful when page URLs themselves are article URLs, or when you want to process paginated pages directly
// baseURL: the base URL (e.g., "https://site.com")
// pagePattern: the pattern for page URLs with %d placeholder (e.g., "/page/%d" or "/page-bla-blah/%d")
// numberOfPages: number of pages to generate (0 = unlimited, check existence for each page)
func DirectPagePipelineBuilder(dbClient *db.Client, baseURL, pagePattern string, pagesPerBatch, pageGenWorkers, contentWorkers, numberOfPages int, extractor urls.URLExtractor) *Pipeline {
	// Step 1: Generate page URLs (uses Generator, not Fetcher)
	step1 := PipelineStep{
		Name:        "Page Range Generator",
		WorkerCount: pageGenWorkers,
		Generator:   NewPageRangeGenerator(baseURL, pagePattern, pagesPerBatch, numberOfPages, extractor),
		Fetcher:     nil, // First step uses Generator
	}

	consumer := ContentConsumer{
		WorkerCount:      contentWorkers,
		ContentProcessor: NewHTTPContentProcessor(),
		ContentSaver:     NewDBContentSaver(dbClient),
	}

	return NewPipeline([]PipelineStep{step1}, consumer)
}

// MultiLevelPipelineBuilder builds a custom pipeline with multiple steps
// Example: BaseURL → [Step 1] → [Step 2] → ... → [Content Consumer]
func MultiLevelPipelineBuilder(steps []PipelineStep, consumer ContentConsumer) *Pipeline {
	return NewPipeline(steps, consumer)
}
