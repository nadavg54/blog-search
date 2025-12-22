package urls

import (
	"fmt"
	"testing"
)

func TestHTMLFetcher_FetchSERadioPage(t *testing.T) {
	// Test fetching URLs from se-radio.net page 1
	fetcher := NewHTMLFetcher(ExtractSERadioURLs)
	
	url := "https://se-radio.net/page/1"
	urls, err := fetcher.Fetch(url)
	if err != nil {
		t.Fatalf("Failed to fetch URLs from %s: %v", url, err)
	}

	if len(urls) == 0 {
		t.Fatal("Expected to find URLs, but got none")
	}

	fmt.Printf("Found %d URLs from %s:\n", len(urls), url)
	for i, u := range urls {
		fmt.Printf("%d. %s - %s\n", i+1, u.Title, u.Location)
	}
}

