package pipeline

import (
	"blog-search/pkg/db"
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
func PaginationPipelineBuilder(dbClient *db.Client, baseURL, pagePattern string, pagesPerBatch, pageGenWorkers, htmlFetcherWorkers, contentWorkers int, extractor urls.URLExtractor, filters ...urls.UrlFilter) *Pipeline {
	// Step 1: Generate page URLs (uses Generator, not Fetcher)
	step1 := PipelineStep{
		Name:        "Page Range Generator",
		WorkerCount: pageGenWorkers,
		Generator:   NewPageRangeGenerator(baseURL, pagePattern, pagesPerBatch, extractor),
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

// MultiLevelPipelineBuilder builds a custom pipeline with multiple steps
// Example: BaseURL → [Step 1] → [Step 2] → ... → [Content Consumer]
func MultiLevelPipelineBuilder(steps []PipelineStep, consumer ContentConsumer) *Pipeline {
	return NewPipeline(steps, consumer)
}
