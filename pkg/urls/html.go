package urls

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"blog-search/pkg/httpclient"

	"github.com/PuerkitoBio/goquery"
)

// URLExtractor is a function type that extracts URLs from HTML content
type URLExtractor func(html string) ([]URL, error)

// HTMLFetcher handles fetching HTML pages and extracting URLs using a provided extractor
type HTMLFetcher struct {
	client     *httpclient.HTTPClient
	extractor  URLExtractor
	clientType httpclient.ClientType
}

// NewHTMLFetcher creates a new HTML fetcher with the given extractor function
// Uses CloudflareClient by default to avoid 403 errors from Cloudflare-protected sites
func NewHTMLFetcher(extractor URLExtractor) *HTMLFetcher {
	return NewHTMLFetcherWithClient(extractor, httpclient.CloudflareClient)
}

// NewHTMLFetcherWithClient creates a new HTML fetcher with a specific client type
func NewHTMLFetcherWithClient(extractor URLExtractor, clientType httpclient.ClientType) *HTMLFetcher {
	return &HTMLFetcher{
		client:     httpclient.NewClient(clientType),
		extractor:  extractor,
		clientType: clientType,
	}
}

// Fetch implements URLsFetcher interface - fetches HTML from the given URL and extracts URLs
func (f *HTMLFetcher) Fetch(url string) ([]URL, error) {
	html, err := f.fetchHTML(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch HTML: %w", err)
	}

	urls, err := f.extractURLsFromHTML(html)
	if err != nil {
		return nil, fmt.Errorf("failed to extract URLs: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs found in HTML")
	}

	return urls, nil
}

// fetchHTML fetches the HTML content from the given URL
func (f *HTMLFetcher) fetchHTML(url string) (string, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// extractURLsFromHTML extracts URLs from HTML using the configured extractor
func (f *HTMLFetcher) extractURLsFromHTML(html string) ([]URL, error) {
	if f.extractor == nil {
		return nil, fmt.Errorf("extractor function is not set")
	}

	return f.extractor(html)
}

// ExtractSERadioURLs extracts article URLs from se-radio.net HTML pages
// It looks for articles in the div with class "col-12 megaphone-order-1 col-lg-8"
// and extracts links from h2.entry-title > a elements
func ExtractSERadioURLs(html string) ([]URL, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var urls []URL

	// Find the container div with class "col-12 megaphone-order-1 col-lg-8"
	doc.Find("div.col-12.megaphone-order-1.col-lg-8").Each(func(i int, container *goquery.Selection) {
		// Find all article elements within the container
		container.Find("article.megaphone-item.megaphone-post").Each(func(j int, article *goquery.Selection) {
			// Find the h2.entry-title > a link
			link := article.Find("h2.entry-title a")
			if link.Length() == 0 {
				return
			}

			href, exists := link.Attr("href")
			if !exists || href == "" {
				return
			}

			title := strings.TrimSpace(link.Text())
			if title == "" {
				// Fallback: try to get title from the link's title attribute or use href
				title, _ = link.Attr("title")
				if title == "" {
					title = href
				}
			}

			urls = append(urls, URL{
				Location: href,
				Title:    title,
			})
		})
	})

	if len(urls) == 0 {
		return nil, fmt.Errorf("no article URLs found in HTML")
	}

	return urls, nil
}
