package parser

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SitemapParser handles sitemap parsing operations
type SitemapParser struct {
	client *http.Client
}

// NewSitemapParser creates a new sitemap parser
func NewSitemapParser() *SitemapParser {
	return &SitemapParser{
		client: &http.Client{},
	}
}

// ParseFromURL fetches and parses a sitemap from the given URL
func (p *SitemapParser) ParseFromURL(url string) ([]URL, error) {
	resp, err := p.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sitemap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read first few bytes to detect sitemap type
	peekBuffer := make([]byte, 512)
	n, err := resp.Body.Read(peekBuffer)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read sitemap: %w", err)
	}

	content := string(peekBuffer[:n])
	reader := io.MultiReader(strings.NewReader(content), resp.Body)

	// Check if it's a sitemap index (contains <sitemapindex>)
	if strings.Contains(content, "<sitemapindex") || strings.Contains(content, "sitemapindex") {
		sitemapURLs, err := p.parseSitemapIndex(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sitemap index: %w", err)
		}

		if len(sitemapURLs) == 0 {
			return nil, fmt.Errorf("sitemap index contained no sitemap URLs")
		}

		// Parse all sitemaps in the index and combine their entries
		var allURLs []URL
		for _, sitemapURL := range sitemapURLs {
			urls, err := p.ParseFromURL(sitemapURL)
			if err != nil {
				// Log error but continue with other sitemaps
				// TODO: Consider adding a logger or error collection mechanism
				continue
			}
			allURLs = append(allURLs, urls...)
		}

		if len(allURLs) == 0 {
			return nil, fmt.Errorf("no entries found in any sitemap from index")
		}

		return allURLs, nil
	}

	// Otherwise, treat it as a regular sitemap
	return p.parseSitemap(reader)
}

// parseSitemapIndex parses a sitemap index file
func (p *SitemapParser) parseSitemapIndex(reader io.Reader) ([]string, error) {
	var index sitemapIndex
	decoder := xml.NewDecoder(reader)

	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to decode sitemap index XML: %w", err)
	}

	urls := make([]string, 0, len(index.Sitemaps))
	for _, ref := range index.Sitemaps {
		if ref.Location != "" {
			urls = append(urls, ref.Location)
		}
	}

	return urls, nil
}

// parseSitemap parses a regular sitemap XML
func (p *SitemapParser) parseSitemap(reader io.Reader) ([]URL, error) {
	var set urlSet
	decoder := xml.NewDecoder(reader)

	if err := decoder.Decode(&set); err != nil {
		return nil, fmt.Errorf("failed to decode sitemap XML: %w", err)
	}

	urls := make([]URL, 0, len(set.URLs))
	for _, urlEntry := range set.URLs {
		if urlEntry.Location != "" {
			url := URL{
				Location: urlEntry.Location,
				// Title not available in sitemaps, leave empty
			}
			urls = append(urls, url)
		}
	}

	return urls, nil
}

// XML structures for parsing sitemap XML

// urlSet represents a regular sitemap structure
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// urlEntry represents a single URL entry in XML
type urlEntry struct {
	Location   string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	Priority   string `xml:"priority,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
}

// sitemapIndex represents a sitemap index structure
type sitemapIndex struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	Sitemaps []sitemapRef `xml:"sitemap"`
}

// sitemapRef represents a reference to another sitemap in an index
type sitemapRef struct {
	Location string `xml:"loc"`
	LastMod  string `xml:"lastmod,omitempty"`
}

