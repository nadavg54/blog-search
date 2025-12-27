package content

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
)

// Extractor defines an interface for extracting title and text from HTML content
type Extractor interface {
	ExtractTitle(htmlContent string) (string, error)
	ExtractText(htmlContent string) (string, error)
}

// DefaultExtractor implements the Extractor interface using the standard extraction functions
type DefaultExtractor struct{}

// NewDefaultExtractor creates a new default extractor
func NewDefaultExtractor() *DefaultExtractor {
	return &DefaultExtractor{}
}

// ExtractTitle extracts the article title using the default extraction logic
func (e *DefaultExtractor) ExtractTitle(htmlContent string) (string, error) {
	return ExtractTitle(htmlContent)
}

// ExtractText extracts the article text using the default extraction logic
func (e *DefaultExtractor) ExtractText(htmlContent string) (string, error) {
	return ExtractText(htmlContent)
}

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

// ExtractTranscript extracts transcript text from HTML content
// It looks for a div with id="transcriptTab" and extracts text from all
// elements with class "transcriptUtterance"
func ExtractTranscript(htmlContent string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Find the transcript tab div
	transcriptDiv := doc.Find("#transcriptTab")
	if transcriptDiv.Length() == 0 {
		return "", fmt.Errorf("transcript tab not found in HTML")
	}

	// Extract all transcript utterances
	var transcriptParts []string
	transcriptDiv.Find("a.transcriptUtterance").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			transcriptParts = append(transcriptParts, text)
		}
	})

	if len(transcriptParts) == 0 {
		return "", fmt.Errorf("no transcript utterances found")
	}

	// Join all parts with spaces
	transcript := strings.Join(transcriptParts, " ")
	return strings.TrimSpace(transcript), nil
}

// DataEngineeringPodcastExtractor implements the Extractor interface
// for dataengineeringpodcast.com pages, using ExtractTranscript for text extraction
type DataEngineeringPodcastExtractor struct{}

// NewDataEngineeringPodcastExtractor creates a new extractor for dataengineeringpodcast.com
func NewDataEngineeringPodcastExtractor() *DataEngineeringPodcastExtractor {
	return &DataEngineeringPodcastExtractor{}
}

// ExtractTitle extracts the article title using the default extraction logic
func (e *DataEngineeringPodcastExtractor) ExtractTitle(htmlContent string) (string, error) {
	return ExtractTitle(htmlContent)
}

// ExtractText extracts the transcript text using ExtractTranscript
// Falls back to default ExtractText if transcript extraction fails
func (e *DataEngineeringPodcastExtractor) ExtractText(htmlContent string) (string, error) {
	// Try to extract transcript first
	transcript, err := ExtractTranscript(htmlContent)
	if err == nil && transcript != "" {
		return transcript, nil
	}
	// Fallback to default text extraction if transcript extraction fails
	return ExtractText(htmlContent)
}
