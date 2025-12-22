package content

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestExtractContentFromSERadioArticle(t *testing.T) {
	// Test fetching and extracting content from a specific se-radio.net article
	articleURL := "https://se-radio.net/2025/11/se-radio-696-flavia-saldanha-on-data-engineering-for-ai/"

	// Fetch HTML
	client := &http.Client{}
	resp, err := client.Get(articleURL)
	if err != nil {
		t.Fatalf("Failed to fetch article: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected status code: %d", resp.StatusCode)
	}

	// Read HTML content
	htmlBytes := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			htmlBytes = append(htmlBytes, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	htmlContent := string(htmlBytes)

	// Extract title
	title, err := ExtractTitle(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract title: %v", err)
	}

	// Extract text content
	text, err := ExtractText(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract text: %v", err)
	}

	// Print results
	fmt.Printf("\n=== Article Content Extraction Test ===\n")
	fmt.Printf("URL: %s\n", articleURL)
	fmt.Printf("\nTitle: %s\n", title)
	fmt.Printf("\nText Content Length: %d characters\n", len(text))
	fmt.Printf("\nFirst 500 characters of text:\n%s\n", text[:min(20000, len(text))])
	fmt.Printf("\nLast 500 characters of text:\n%s\n", text[max(0, len(text)-500):])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Test using direct HTTP fetch and content extraction
func TestExtractContentUsingDirectFetch(t *testing.T) {
	articleURL := "https://se-radio.net/2025/11/se-radio-696-flavia-saldanha-on-data-engineering-for-ai/"

	// Fetch HTML directly
	client := &http.Client{}
	resp, err := client.Get(articleURL)
	if err != nil {
		t.Fatalf("Failed to fetch article: %v", err)
	}
	defer resp.Body.Close()

	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	htmlContent := string(htmlBytes)

	// Extract title and text
	title, err := ExtractTitle(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract title: %v", err)
	}

	text, err := ExtractText(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract text: %v", err)
	}

	fmt.Printf("\n=== Using Direct HTTP Fetch ===\n")
	fmt.Printf("Title: %s\n", title)
	fmt.Printf("Text Length: %d characters\n", len(text))
	if len(text) > 0 {
		fmt.Printf("First 300 chars: %s\n", text[:min(300, len(text))])
	}
}
