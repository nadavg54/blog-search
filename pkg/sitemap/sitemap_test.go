package sitemap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSitemap(t *testing.T) {
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

	parser := NewParser()
	reader := strings.NewReader(xmlData)

	entries, err := parser.parseSitemap(reader)
	if err != nil {
		t.Fatalf("Failed to parse sitemap: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Check first entry (with all fields)
	entry1 := entries[0]
	if entry1.Location != "https://engineering.fb.com/post1" {
		t.Errorf("Expected location 'https://engineering.fb.com/post1', got '%s'", entry1.Location)
	}
	if entry1.LastMod != "2024-01-15" {
		t.Errorf("Expected LastMod '2024-01-15', got '%s'", entry1.LastMod)
	}
	if entry1.Priority != "0.8" {
		t.Errorf("Expected Priority '0.8', got '%s'", entry1.Priority)
	}
	if entry1.ChangeFreq != "monthly" {
		t.Errorf("Expected ChangeFreq 'monthly', got '%s'", entry1.ChangeFreq)
	}

	// Check second entry (partial fields)
	entry2 := entries[1]
	if entry2.Location != "https://engineering.fb.com/post2" {
		t.Errorf("Expected location 'https://engineering.fb.com/post2', got '%s'", entry2.Location)
	}
	if entry2.LastMod != "2024-01-20" {
		t.Errorf("Expected LastMod '2024-01-20', got '%s'", entry2.LastMod)
	}

	// Check third entry (only location)
	entry3 := entries[2]
	if entry3.Location != "https://engineering.fb.com/post3" {
		t.Errorf("Expected location 'https://engineering.fb.com/post3', got '%s'", entry3.Location)
	}
}

func TestParseSitemapIndex(t *testing.T) {
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

	parser := NewParser()
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

func TestParseSitemapEmpty(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`

	parser := NewParser()
	reader := strings.NewReader(xmlData)

	entries, err := parser.parseSitemap(reader)
	if err != nil {
		t.Fatalf("Failed to parse empty sitemap: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

func TestParseSitemapInvalidXML(t *testing.T) {
	invalidXML := `<?xml version="1.0"?><invalid>`

	parser := NewParser()
	reader := strings.NewReader(invalidXML)

	_, err := parser.parseSitemap(reader)
	if err == nil {
		t.Error("Expected error for invalid XML, got nil")
	}
}

func TestParseFromURL_SitemapIndex(t *testing.T) {
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

	parser := NewParser()
	entries, err := parser.ParseFromURL(server.URL + "/sitemap-index.xml")
	if err != nil {
		t.Fatalf("Failed to parse sitemap index from URL: %v", err)
	}

	// Should have entries from both sitemaps
	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries (2 from each sitemap), got %d", len(entries))
	}

	// Verify we got entries from both sitemaps
	locations := make(map[string]bool)
	for _, entry := range entries {
		locations[entry.Location] = true
	}

	expectedLocations := []string{
		"https://example.com/article1",
		"https://example.com/article2",
		"https://example.com/article3",
		"https://example.com/article4",
	}

	for _, expected := range expectedLocations {
		if !locations[expected] {
			t.Errorf("Expected to find location '%s' in entries", expected)
		}
	}
}
