package urls

import (
	"os"
	"testing"
)

func TestFileParser_ParseFromURL(t *testing.T) {
	// Create a temporary file with URLs
	file, err := os.CreateTemp("", "test-urls-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	// Write test URLs to file
	testContent := `https://example.com/article1
https://example.com/article2
https://example.com/article3

# This is a comment
https://example.com/article4
`
	if _, err := file.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	file.Close()

	parser := NewFileParser()
	urls, err := parser.Fetch(file.Name())
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	if len(urls) != 4 {
		t.Fatalf("Expected 4 URLs, got %d", len(urls))
	}

	expectedURLs := []string{
		"https://example.com/article1",
		"https://example.com/article2",
		"https://example.com/article3",
		"https://example.com/article4",
	}

	for i, expected := range expectedURLs {
		if urls[i].Location != expected {
			t.Errorf("Expected URL %d to be '%s', got '%s'", i, expected, urls[i].Location)
		}
		if urls[i].Title != "" {
			t.Errorf("Expected empty title for URL %d, got '%s'", i, urls[i].Title)
		}
	}
}

func TestFileParser_ParseFromURL_EmptyFile(t *testing.T) {
	// Create an empty temporary file
	file, err := os.CreateTemp("", "test-empty-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	file.Close()

	parser := NewFileParser()
	_, err = parser.Fetch(file.Name())
	if err == nil {
		t.Error("Expected error for empty file, got nil")
	}
}

func TestFileParser_ParseFromURL_NonexistentFile(t *testing.T) {
	parser := NewFileParser()
	_, err := parser.Fetch("/nonexistent/file/path.txt")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestFileParser_ParseFromURL_WithComments(t *testing.T) {
	// Create a temporary file with URLs and comments
	file, err := os.CreateTemp("", "test-comments-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	testContent := `# This is a comment
https://example.com/article1
# Another comment
https://example.com/article2
`
	if _, err := file.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	file.Close()

	parser := NewFileParser()
	urls, err := parser.Fetch(file.Name())
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// Should only have 2 URLs, comments should be skipped
	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs (comments should be skipped), got %d", len(urls))
	}
}
