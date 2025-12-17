package content

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// ExtractTitle extracts the article title from HTML content with fallback mechanisms
func ExtractTitle(htmlContent string) (string, error) {
	// Try readability first
	article, err := readability.FromReader(strings.NewReader(htmlContent), nil)
	if err == nil {
		title := strings.TrimSpace(article.Title)
		if title != "" {
			return title, nil
		}
	}

	// Fallback: Try parsing HTML directly with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Try <title> tag
	if title := strings.TrimSpace(doc.Find("title").First().Text()); title != "" {
		return title, nil
	}

	// Try <h1> tag (often the main heading)
	if title := strings.TrimSpace(doc.Find("h1").First().Text()); title != "" {
		return title, nil
	}

	// Try meta property="og:title"
	if title, exists := doc.Find("meta[property='og:title']").Attr("content"); exists && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title), nil
	}

	// Try meta name="title"
	if title, exists := doc.Find("meta[name='title']").Attr("content"); exists && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title), nil
	}

	return "", fmt.Errorf("title not found in HTML")
}
