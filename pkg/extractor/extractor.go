package extractor

import (
	"fmt"
	"strings"

	"github.com/go-shiori/go-readability"
)

// ExtractText extracts the main article text from HTML content
func ExtractText(htmlContent string) (string, error) {
	article, err := readability.FromReader(strings.NewReader(htmlContent), nil)
	if err != nil {
		return "", fmt.Errorf("failed to extract text: %w", err)
	}

	return strings.TrimSpace(article.TextContent), nil
}

// ExtractTitle extracts the article title from HTML content
func ExtractTitle(htmlContent string) (string, error) {
	article, err := readability.FromReader(strings.NewReader(htmlContent), nil)
	if err != nil {
		return "", fmt.Errorf("failed to extract title: %w", err)
	}

	title := strings.TrimSpace(article.Title)
	if title == "" {
		return "", fmt.Errorf("title not found in HTML")
	}

	return title, nil
}
