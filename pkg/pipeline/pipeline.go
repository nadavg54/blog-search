package pipeline

import (
	"context"
	"fmt"
	"log"
	"sync"

	"blog-search/pkg/domain"
)

// URLGenerator generates initial URLs (used for the first step only)
// Examples: PageRangeGenerator generates paginated page URLs
type URLGenerator interface {
	// Generate generates URLs from configuration/pattern
	// Does not take input URL - generates URLs based on internal configuration
	Generate(ctx context.Context) ([]string, error)
}

// URLFetcher extracts URLs from a given URL (used for subsequent steps)
// Examples: RSS/Sitemap fetchers, HTML page fetchers
type URLFetcher interface {
	// Fetch extracts URLs from the given URL
	// For base URL fetchers (RSS/Sitemap), this is called once with the base URL
	// For intermediate fetchers, this is called for each URL from the previous step
	// Filters are applied internally by the fetcher implementation
	Fetch(ctx context.Context, url string) ([]string, error)
}

// ContentProcessor processes a URL and returns an Article
// Handles fetching HTML, extracting content, and creating the article
type ContentProcessor interface {
	// ProcessContent fetches content from a URL and returns an Article
	ProcessContent(ctx context.Context, url string) (*domain.Article, error)
}

// ContentSaver saves an Article to a storage backend
type ContentSaver interface {
	// SaveArticle saves an article to storage
	SaveArticle(ctx context.Context, article *domain.Article) error
}

// PipelineStep represents a step in the pipeline that extracts URLs
// First step can use either URLGenerator or URLFetcher (with baseURL)
// Subsequent steps use URLFetcher
type PipelineStep struct {
	Name        string
	WorkerCount int
	Generator   URLGenerator // Used for first step (optional)
	Fetcher     URLFetcher   // Used for all steps (required if Generator is nil)
}

// ContentConsumer is the final step that fetches content and saves to storage
type ContentConsumer struct {
	WorkerCount      int
	ContentProcessor ContentProcessor // Processor to fetch and extract content
	ContentSaver     ContentSaver     // Saver to persist articles
}

// Pipeline orchestrates multiple steps and a final content consumer
type Pipeline struct {
	steps           []PipelineStep
	contentConsumer ContentConsumer
}

// NewPipeline creates a new pipeline with the given steps and content consumer
func NewPipeline(steps []PipelineStep, consumer ContentConsumer) *Pipeline {
	return &Pipeline{
		steps:           steps,
		contentConsumer: consumer,
	}
}

// Run executes the pipeline:
// 1. First step: uses Generator (if set) or Fetcher with baseURL
// 2. Each subsequent step extracts URLs and passes them to the next step
// 3. Final step passes URLs to content consumer
// 4. Content consumer fetches content and saves to database
func (p *Pipeline) Run(ctx context.Context, baseURL string) error {
	if len(p.steps) == 0 {
		return fmt.Errorf("pipeline has no steps")
	}

	channels, contentChan := p.createChannels()
	var wg sync.WaitGroup

	p.startAllWorkers(ctx, baseURL, channels, contentChan, &wg)
	wg.Wait()

	return nil
}

// createChannels creates channels for communication between pipeline steps
func (p *Pipeline) createChannels() ([]chan string, chan string) {
	channels := make([]chan string, len(p.steps))
	for i := range channels {
		bufferSize := p.steps[i].WorkerCount * 2
		if i == 0 {
			bufferSize = 100 // First channel needs more buffer
		}
		channels[i] = make(chan string, bufferSize)
	}

	contentChan := make(chan string, p.contentConsumer.WorkerCount*2)
	return channels, contentChan
}

// startAllWorkers starts all workers in the pipeline
func (p *Pipeline) startAllWorkers(ctx context.Context, baseURL string, channels []chan string, contentChan chan string, wg *sync.WaitGroup) {
	p.startContentConsumer(ctx, contentChan, wg)
	p.startSubsequentStepWorkers(ctx, channels, contentChan, wg)
	p.startFirstStepWorker(ctx, baseURL, channels, contentChan, wg)
}

// startSubsequentStepWorkers starts workers for all steps after the first
func (p *Pipeline) startSubsequentStepWorkers(ctx context.Context, channels []chan string, contentChan chan string, wg *sync.WaitGroup) {
	for i := 1; i < len(p.steps); i++ {
		inputChan := channels[i-1]
		outputChan := p.getOutputChannelForStep(i, channels, contentChan)
		p.startStepWorkers(ctx, p.steps[i], inputChan, outputChan, wg)
	}
}

// getOutputChannelForStep returns the appropriate output channel for a step
func (p *Pipeline) getOutputChannelForStep(stepIndex int, channels []chan string, contentChan chan string) chan string {
	if stepIndex == len(p.steps)-1 {
		return contentChan
	}
	return channels[stepIndex]
}

// startFirstStepWorker starts the first step worker
func (p *Pipeline) startFirstStepWorker(ctx context.Context, baseURL string, channels []chan string, contentChan chan string, wg *sync.WaitGroup) {
	firstStepOutput := channels[0]
	if len(p.steps) == 1 {
		firstStepOutput = contentChan
	}
	p.startFirstStep(ctx, p.steps[0], baseURL, firstStepOutput, wg)
}

// startFirstStep starts the first step (can use Generator or Fetcher with baseURL)
func (p *Pipeline) startFirstStep(ctx context.Context, step PipelineStep, baseURL string, outputChan chan<- string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(outputChan)

		urls, err := p.generateOrFetchURLs(ctx, step, baseURL)
		if err != nil {
			return
		}

		p.sendURLsToChannel(ctx, urls, outputChan, "First step")
	}()
}

// generateOrFetchURLs generates or fetches URLs for the first step
func (p *Pipeline) generateOrFetchURLs(ctx context.Context, step PipelineStep, baseURL string) ([]string, error) {
	if step.Generator != nil {
		urls, err := step.Generator.Generate(ctx)
		if err != nil {
			log.Printf("First step (Generator): Error generating URLs: %v", err)
			return nil, err
		}
		return urls, nil
	}

	if step.Fetcher != nil {
		urls, err := step.Fetcher.Fetch(ctx, baseURL)
		if err != nil {
			log.Printf("First step (Fetcher): Error fetching URLs from %s: %v", baseURL, err)
			return nil, err
		}
		return urls, nil
	}

	log.Printf("First step: Neither Generator nor Fetcher is set")
	return nil, fmt.Errorf("neither generator nor fetcher is set")
}

// sendURLsToChannel sends URLs to the output channel with logging
func (p *Pipeline) sendURLsToChannel(ctx context.Context, urls []string, outputChan chan<- string, stepName string) {
	log.Printf("%s: Sending %d URLs to next step", stepName, len(urls))
	for i, url := range urls {
		select {
		case outputChan <- url:
			if i < 5 || i == len(urls)-1 {
				log.Printf("%s: Sent URL %d/%d: %s", stepName, i+1, len(urls), url)
			}
		case <-ctx.Done():
			log.Printf("%s: Context cancelled", stepName)
			return
		}
	}
	log.Printf("%s: Generated/fetched %d URLs, all sent", stepName, len(urls))
}

// startStepWorkers starts workers for a pipeline step (subsequent steps)
func (p *Pipeline) startStepWorkers(ctx context.Context, step PipelineStep, inputChan <-chan string, outputChan chan<- string, wg *sync.WaitGroup) {
	if step.Fetcher == nil {
		log.Printf("Step %s: Fetcher is not set", step.Name)
		return
	}

	var stepWg sync.WaitGroup

	for i := 0; i < step.WorkerCount; i++ {
		stepWg.Add(1)
		wg.Add(1)
		go p.startStepWorker(ctx, step, i, inputChan, outputChan, &stepWg, wg)
	}

	go p.closeChannelWhenDone(&stepWg, outputChan)
}

// startStepWorker starts a single worker for a pipeline step
func (p *Pipeline) startStepWorker(ctx context.Context, step PipelineStep, workerID int, inputChan <-chan string, outputChan chan<- string, stepWg, wg *sync.WaitGroup) {
	defer stepWg.Done()
	defer wg.Done()

	for {
		select {
		case url, ok := <-inputChan:
			if !ok {
				return
			}
			p.processURLInStep(ctx, step, workerID, url, outputChan)

		case <-ctx.Done():
			log.Printf("Step %s (worker %d): Context cancelled", step.Name, workerID)
			return
		}
	}
}

// processURLInStep processes a URL in a pipeline step: fetches URLs and sends them to output
func (p *Pipeline) processURLInStep(ctx context.Context, step PipelineStep, workerID int, url string, outputChan chan<- string) {
	log.Printf("Step %s (worker %d): Fetching URLs from %s", step.Name, workerID, url)
	extractedURLs, err := step.Fetcher.Fetch(ctx, url)
	if err != nil {
		log.Printf("Step %s (worker %d): Error fetching URLs from %s: %v", step.Name, workerID, url, err)
		return
	}

	log.Printf("Step %s (worker %d): Extracted %d URLs from %s", step.Name, workerID, len(extractedURLs), url)
	p.sendExtractedURLs(ctx, step, workerID, extractedURLs, outputChan)
}

// sendExtractedURLs sends extracted URLs to the output channel
func (p *Pipeline) sendExtractedURLs(ctx context.Context, step PipelineStep, workerID int, extractedURLs []string, outputChan chan<- string) {
	for i, extractedURL := range extractedURLs {
		select {
		case outputChan <- extractedURL:
			if i < 3 || i == len(extractedURLs)-1 {
				log.Printf("Step %s (worker %d): Sent URL %d/%d: %s", step.Name, workerID, i+1, len(extractedURLs), extractedURL)
			}
		case <-ctx.Done():
			log.Printf("Step %s (worker %d): Context cancelled", step.Name, workerID)
			return
		}
	}
}

// closeChannelWhenDone closes the output channel when all step workers are done
func (p *Pipeline) closeChannelWhenDone(stepWg *sync.WaitGroup, outputChan chan<- string) {
	stepWg.Wait()
	close(outputChan)
}

// startContentConsumer starts the content consumer workers
func (p *Pipeline) startContentConsumer(ctx context.Context, inputChan <-chan string, wg *sync.WaitGroup) {
	for i := 0; i < p.contentConsumer.WorkerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case url, ok := <-inputChan:
					if !ok {
						// Channel closed, no more URLs
						return
					}

					// Process this URL: fetch content and save to database
					log.Printf("Content worker %d: Starting to process URL: %s", workerID, url)
					if err := p.processContentURL(ctx, url); err != nil {
						log.Printf("Content worker %d: ERROR processing URL %s: %v", workerID, url, err)
					} else {
						log.Printf("Content worker %d: SUCCESS - Processed and saved URL: %s", workerID, url)
					}

				case <-ctx.Done():
					log.Printf("Content worker %d: Context cancelled", workerID)
					return
				}
			}
		}(i)
	}
}

// processContentURL processes a URL using the content processor and saves it using the content saver
func (p *Pipeline) processContentURL(ctx context.Context, url string) error {
	if p.contentConsumer.ContentProcessor == nil {
		return fmt.Errorf("content processor is not set")
	}
	if p.contentConsumer.ContentSaver == nil {
		return fmt.Errorf("content saver is not set")
	}

	// Process content (fetch, extract, create article)
	log.Printf("processContentURL: Fetching and extracting content from %s", url)
	article, err := p.contentConsumer.ContentProcessor.ProcessContent(ctx, url)
	if err != nil {
		log.Printf("processContentURL: ERROR processing content from %s: %v", url, err)
		return fmt.Errorf("failed to process content: %w", err)
	}

	log.Printf("processContentURL: Successfully extracted article - Title: %s, URL: %s", article.Title, article.URL)

	// Save article
	log.Printf("processContentURL: Saving article to database - URL: %s", article.URL)
	if err := p.contentConsumer.ContentSaver.SaveArticle(ctx, article); err != nil {
		log.Printf("processContentURL: ERROR saving article to database - URL: %s, Error: %v", article.URL, err)
		return fmt.Errorf("failed to save article: %w", err)
	}

	log.Printf("processContentURL: SUCCESS - Article saved to database - URL: %s", article.URL)
	return nil
}
