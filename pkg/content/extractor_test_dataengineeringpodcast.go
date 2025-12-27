package content

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractTextFromDataEngineeringPodcast tests if the current ExtractText function
// can extract transcript text from the dataengineeringpodcast.html file
func TestExtractTextFromDataEngineeringPodcast(t *testing.T) {
	// Read the HTML file
	htmlPath := filepath.Join("..", "..", "html-page-examples", "dataengineeringpodcast.html")
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read HTML file: %v", err)
	}

	htmlContent := string(htmlBytes)

	// Extract title
	title, err := ExtractTitle(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract title: %v", err)
	}

	// Extract text content using current extractor
	text, err := ExtractText(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract text: %v", err)
	}

	// Print results to see what we got
	fmt.Printf("\n=== Data Engineering Podcast Extraction Test ===\n")
	fmt.Printf("Title: %s\n", title)
	fmt.Printf("Text Content Length: %d characters\n", len(text))
	
	if len(text) > 0 {
		previewLen := min(1000, len(text))
		fmt.Printf("\nFirst %d characters of extracted text:\n%s\n", previewLen, text[:previewLen])
		
		// Check if transcript content is present
		// Look for some known transcript phrases
		knownPhrases := []string{
			"Hello, and welcome to the Data Engineering podcast",
			"Tobias Macey",
			"Nick Schrock",
		}
		
		fmt.Printf("\n=== Checking for transcript content ===\n")
		foundCount := 0
		for _, phrase := range knownPhrases {
			if strings.Contains(text, phrase) {
				fmt.Printf("✓ Found phrase: '%s'\n", phrase)
				foundCount++
			} else {
				fmt.Printf("✗ Missing phrase: '%s'\n", phrase)
			}
		}
		
		if foundCount == 0 {
			fmt.Printf("\n⚠️  No transcript phrases found - current extractor may not be extracting transcript content\n")
		}
	} else {
		fmt.Printf("\n⚠️  No text extracted!\n")
	}
}

// TestExtractTranscriptFromDataEngineeringPodcast tests the specialized ExtractTranscript function
// to verify it correctly extracts transcript text from the dataengineeringpodcast.html file
func TestExtractTranscriptFromDataEngineeringPodcast(t *testing.T) {
	// Read the HTML file
	htmlPath := filepath.Join("..", "..", "html-page-examples", "dataengineeringpodcast.html")
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read HTML file: %v", err)
	}

	htmlContent := string(htmlBytes)

	// Extract transcript using specialized function
	transcript, err := ExtractTranscript(htmlContent)
	if err != nil {
		t.Fatalf("Failed to extract transcript: %v", err)
	}

	// Verify transcript was extracted
	if len(transcript) == 0 {
		t.Fatal("Extracted transcript is empty")
	}

	// Print results
	fmt.Printf("\n=== Transcript Extraction Test ===\n")
	fmt.Printf("Transcript Length: %d characters\n", len(transcript))
	
	previewLen := min(500, len(transcript))
	fmt.Printf("\nFirst %d characters of transcript:\n%s\n", previewLen, transcript[:previewLen])
	
	// Check for known transcript phrases that should be in the transcript
	knownPhrases := []string{
		"Hello, and welcome to the Data Engineering podcast",
		"Data teams everywhere face the same problem",
		"Nick Schrock",
	}
	
	fmt.Printf("\n=== Verifying transcript content ===\n")
	foundCount := 0
	for _, phrase := range knownPhrases {
		if strings.Contains(transcript, phrase) {
			fmt.Printf("✓ Found phrase: '%s'\n", phrase)
			foundCount++
		} else {
			fmt.Printf("✗ Missing phrase: '%s'\n", phrase)
		}
	}
	
	// Verify at least some key phrases are found
	if foundCount == 0 {
		t.Error("No expected transcript phrases found - extraction may have failed")
	}
	
	// Verify transcript has reasonable length (should be substantial for a podcast)
	if len(transcript) < 1000 {
		t.Errorf("Transcript seems too short (%d chars), expected at least 1000 characters", len(transcript))
	}
}

