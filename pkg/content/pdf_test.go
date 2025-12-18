package content

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractTextFromPDFFile_SED754TiDB verifies we can extract non-empty, meaningful text
// from the SED754 TiDB.pdf sample document.
func TestExtractTextFromPDFFile_SED754TiDB(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "SED754 TiDB.pdf")
	text, err := ExtractTextFromPDFFile(pdfPath)
	if err != nil {
		t.Fatalf("ExtractTextFromPDFFile(%q) returned error: %v", pdfPath, err)
	}

	if len(text) == 0 {
		t.Fatalf("expected non-empty extracted text for %q", pdfPath)
	}

	println(text)

	const expectedSnippet = "TiDB"
	if !strings.Contains(text, expectedSnippet) {
		t.Fatalf("expected extracted text for %q to contain %q, got (first 200 chars): %q",
			pdfPath, expectedSnippet, firstNRunes(text, 200))
	}
}

// firstNRunes returns at most the first n runes from s, preserving UTF-8 correctness.
func firstNRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
