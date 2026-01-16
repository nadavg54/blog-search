package main

import (
	"log"
	"strings"

	"blog-search/pkg/httpclient"
	"blog-search/pkg/urls"
)

// logPipelineConfig logs the configuration for sitemap/RSS pipelines
func logPipelineConfig(pipelineType string, urlFetcherWorkers, contentWorkers int, filters []urls.UrlFilter) {
	log.Printf("Running %s pipeline with %d URL fetcher workers, %d content workers", pipelineType, urlFetcherWorkers, contentWorkers)
	if len(filters) > 0 {
		log.Printf("Applied %d URL filter(s)", len(filters))
	}
}

// logPaginationConfig logs the configuration for pagination pipelines
func logPaginationConfig(baseURL, pagePattern, extractorType string, pagesPerBatch, pageGenWorkers, urlFetcherWorkers, contentWorkers, numberOfPages int, filters []urls.UrlFilter) {
	log.Printf("Running Pagination pipeline for %s with pattern %s:", baseURL, pagePattern)

	extractorName := extractorType
	if extractorName == "" {
		if strings.Contains(baseURL, "dataengineeringpodcast.com") {
			extractorName = "data-engineering-podcast (auto-detected)"
		} else {
			extractorName = "se-radio (default)"
		}
	}

	log.Printf("  Extractor: %s", extractorName)
	log.Printf("  Pages per batch: %d", pagesPerBatch)
	log.Printf("  Page Generator Workers: %d", pageGenWorkers)
	log.Printf("  URL Fetcher Workers: %d", urlFetcherWorkers)
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

// logCategoryConfig logs the configuration for category pipelines
func logCategoryConfig(baseURL string, categories []string, extractorType string, urlFetcherWorkers, contentWorkers int, filters []urls.UrlFilter) {
	log.Printf("Running Category pipeline for %s with categories: %v", baseURL, categories)

	extractorName := extractorType
	if extractorName == "" {
		if strings.Contains(baseURL, "dataengineeringpodcast.com") {
			extractorName = "data-engineering-podcast (auto-detected)"
		} else {
			extractorName = "se-radio (default)"
		}
	}

	log.Printf("  Extractor: %s", extractorName)
	log.Printf("  URL Fetcher Workers: %d", urlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	if len(filters) > 0 {
		log.Printf("  Applied %d URL filter(s)", len(filters))
	}
}

// logHTMLFileConfig logs the configuration for HTML file pipelines
func logHTMLFileConfig(htmlFilePath, extractorType string, urlFetcherWorkers, contentWorkers int, clientType httpclient.ClientType, filters []urls.UrlFilter, baseURL string) {
	log.Printf("Running HTML File pipeline for file: %s", htmlFilePath)
	log.Printf("  Extractor: %s", extractorType)
	if baseURL != "" {
		log.Printf("  Base URL: %s (for resolving relative URLs)", baseURL)
	}
	log.Printf("  URL Fetcher Workers: %d", urlFetcherWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	log.Printf("  HTTP Client Type: %s", clientType)
	if len(filters) > 0 {
		log.Printf("  Applied %d URL filter(s)", len(filters))
	}
}

// logDirectPageConfig logs the configuration for direct page pipelines
func logDirectPageConfig(baseURL, pagePattern, extractorType string, pagesPerBatch, pageGenWorkers, contentWorkers, numberOfPages int) {
	log.Printf("Running Direct Page pipeline for %s with pattern %s:", baseURL, pagePattern)

	extractorName := extractorType
	if extractorName == "" {
		if strings.Contains(baseURL, "dataengineeringpodcast.com") {
			extractorName = "data-engineering-podcast (auto-detected)"
		} else {
			extractorName = "se-radio (default)"
		}
	}

	log.Printf("  Extractor: %s", extractorName)
	log.Printf("  Pages per batch: %d", pagesPerBatch)
	log.Printf("  Page Generator Workers: %d", pageGenWorkers)
	log.Printf("  Content Workers: %d", contentWorkers)
	if numberOfPages > 0 {
		log.Printf("  Number of pages: %d (fixed, existence checks bypassed)", numberOfPages)
	} else {
		log.Printf("  Number of pages: unlimited (will check existence for each page)")
	}
	log.Printf("  Note: Page URLs are processed directly as articles (no URL extraction step)")
}

