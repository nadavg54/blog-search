package urls

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSitemapParser_ParseSitemap(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>https://engineering.fb.com/post1</loc>
		<lastmod>2024-01-15</lastmod>
		<priority>0.8</priority>
		<changefreq>monthly</changefreq>
	</url>
	<url>
		<loc>https://engineering.fb.com/post2</loc>
		<lastmod>2024-01-20</lastmod>
	</url>
	<url>
		<loc>https://engineering.fb.com/post3</loc>
	</url>
</urlset>`

	parser := NewSitemapParser()
	reader := strings.NewReader(xmlData)

	urls, err := parser.parseSitemap(reader)
	if err != nil {
		t.Fatalf("Failed to parse sitemap: %v", err)
	}

	if len(urls) != 3 {
		t.Fatalf("Expected 3 URLs, got %d", len(urls))
	}

	// Check first URL
	url1 := urls[0]
	if url1.Location != "https://engineering.fb.com/post1" {
		t.Errorf("Expected location 'https://engineering.fb.com/post1', got '%s'", url1.Location)
	}

	// Check second URL
	url2 := urls[1]
	if url2.Location != "https://engineering.fb.com/post2" {
		t.Errorf("Expected location 'https://engineering.fb.com/post2', got '%s'", url2.Location)
	}

	// Check third URL
	url3 := urls[2]
	if url3.Location != "https://engineering.fb.com/post3" {
		t.Errorf("Expected location 'https://engineering.fb.com/post3', got '%s'", url3.Location)
	}
}

func TestSitemapParser_ParseSitemapIndex(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<sitemap>
		<loc>https://engineering.fb.com/sitemap1.xml</loc>
		<lastmod>2024-01-15</lastmod>
	</sitemap>
	<sitemap>
		<loc>https://engineering.fb.com/sitemap2.xml</loc>
		<lastmod>2024-01-20</lastmod>
	</sitemap>
</sitemapindex>`

	parser := NewSitemapParser()
	reader := strings.NewReader(xmlData)

	urls, err := parser.parseSitemapIndex(reader)
	if err != nil {
		t.Fatalf("Failed to parse sitemap index: %v", err)
	}

	if len(urls) != 2 {
		t.Fatalf("Expected 2 sitemap URLs, got %d", len(urls))
	}

	if urls[0] != "https://engineering.fb.com/sitemap1.xml" {
		t.Errorf("Expected first URL 'https://engineering.fb.com/sitemap1.xml', got '%s'", urls[0])
	}

	if urls[1] != "https://engineering.fb.com/sitemap2.xml" {
		t.Errorf("Expected second URL 'https://engineering.fb.com/sitemap2.xml', got '%s'", urls[1])
	}
}

func TestSitemapParser_ParseSitemapEmpty(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`

	parser := NewSitemapParser()
	reader := strings.NewReader(xmlData)

	urls, err := parser.parseSitemap(reader)
	if err != nil {
		t.Fatalf("Failed to parse empty sitemap: %v", err)
	}

	if len(urls) != 0 {
		t.Errorf("Expected 0 URLs, got %d", len(urls))
	}
}

func TestSitemapParser_ParseSitemapInvalidXML(t *testing.T) {
	invalidXML := `<?xml version="1.0"?><invalid>`

	parser := NewSitemapParser()
	reader := strings.NewReader(invalidXML)

	_, err := parser.parseSitemap(reader)
	if err == nil {
		t.Error("Expected error for invalid XML, got nil")
	}
}

func TestSitemapParser_ParseFromURL_SitemapIndex(t *testing.T) {
	// Create a test HTTP server that serves different sitemaps
	sitemap1 := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>https://example.com/article1</loc>
	</url>
	<url>
		<loc>https://example.com/article2</loc>
	</url>
</urlset>`

	sitemap2 := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<url>
		<loc>https://example.com/article3</loc>
	</url>
	<url>
		<loc>https://example.com/article4</loc>
	</url>
</urlset>`

	// Create test server first
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap-index.xml":
			// Generate sitemap index with actual server URL
			sitemapIndex := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	<sitemap>
		<loc>` + serverURL + `/sitemap1.xml</loc>
	</sitemap>
	<sitemap>
		<loc>` + serverURL + `/sitemap2.xml</loc>
	</sitemap>
</sitemapindex>`
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemapIndex))
		case "/sitemap1.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemap1))
		case "/sitemap2.xml":
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sitemap2))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	parser := NewSitemapParser()
	urls, err := parser.Fetch(server.URL + "/sitemap-index.xml")
	if err != nil {
		t.Fatalf("Failed to parse sitemap index from URL: %v", err)
	}

	// Should have URLs from both sitemaps
	if len(urls) != 4 {
		t.Fatalf("Expected 4 URLs (2 from each sitemap), got %d", len(urls))
	}

	// Verify we got URLs from both sitemaps
	locations := make(map[string]bool)
	for _, url := range urls {
		locations[url.Location] = true
	}

	expectedLocations := []string{
		"https://example.com/article1",
		"https://example.com/article2",
		"https://example.com/article3",
		"https://example.com/article4",
	}

	for _, expected := range expectedLocations {
		if !locations[expected] {
			t.Errorf("Expected to find location '%s' in URLs", expected)
		}
	}
}
