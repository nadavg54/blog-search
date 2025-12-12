package sitemap

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Entry represents a single URL entry from a sitemap
type Entry struct {
	Location   string // URL of the article
	LastMod    string // Last modification date (optional)
	Priority   string // Priority value (optional)
	ChangeFreq string // Change frequency (optional)
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

// Parser handles sitemap parsing operations
type Parser struct {
	client *http.Client
}

// NewParser creates a new sitemap parser
func NewParser() *Parser {
	return &Parser{
		client: &http.Client{},
	}
}

// ParseFromURL fetches and parses a sitemap from the given URL
func (p *Parser) ParseFromURL(sitemapURL string) ([]Entry, error) {
	resp, err := p.client.Get(sitemapURL)
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
		var allEntries []Entry
		for _, sitemapURL := range sitemapURLs {
			entries, err := p.ParseFromURL(sitemapURL)
			if err != nil {
				// Log error but continue with other sitemaps
				// TODO: Consider adding a logger or error collection mechanism
				continue
			}
			allEntries = append(allEntries, entries...)
		}

		if len(allEntries) == 0 {
			return nil, fmt.Errorf("no entries found in any sitemap from index")
		}

		return allEntries, nil
	}

	// Otherwise, treat it as a regular sitemap
	return p.parseSitemap(reader)
}

// parseSitemapIndex parses a sitemap index file
func (p *Parser) parseSitemapIndex(reader io.Reader) ([]string, error) {
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
func (p *Parser) parseSitemap(reader io.Reader) ([]Entry, error) {
	var set urlSet
	decoder := xml.NewDecoder(reader)

	if err := decoder.Decode(&set); err != nil {
		return nil, fmt.Errorf("failed to decode sitemap XML: %w", err)
	}

	entries := make([]Entry, 0, len(set.URLs))
	for _, urlEntry := range set.URLs {
		if urlEntry.Location != "" {
			entry := Entry{
				Location:   urlEntry.Location,
				LastMod:    urlEntry.LastMod,
				Priority:   urlEntry.Priority,
				ChangeFreq: urlEntry.ChangeFreq,
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}
