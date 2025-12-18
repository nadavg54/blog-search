package content

import (
	"bytes"
	"errors"
	"io"

	"github.com/ledongthuc/pdf"
)

var (
	errEmptyPDFPath    = errors.New("pdf path is empty")
	errNilSourceReader = errors.New("pdf source reader is nil")
	errEmptyPDFContent = errors.New("pdf content is empty")
	errNilPDFDocument  = errors.New("pdf document is nil")
)

// ExtractTextFromPDFFile extracts text content from a PDF located at the given filesystem path.
// This is a convenience wrapper around the shared PDF text extraction logic.
func ExtractTextFromPDFFile(path string) (string, error) {
	if path == "" {
		return "", errEmptyPDFPath
	}

	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return extractTextFromPDFDocument(reader)
}

// ExtractTextFromPDFReader extracts text content from a PDF provided via an io.Reader.
// This is intended for use with HTTP response bodies or other in-memory streams.
func ExtractTextFromPDFReader(r io.Reader) (string, error) {
	if r == nil {
		return "", errNilSourceReader
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}

	data := buf.Bytes()
	if len(data) == 0 {
		return "", errEmptyPDFContent
	}

	byteReader := bytes.NewReader(data)
	doc, err := pdf.NewReader(byteReader, int64(len(data)))
	if err != nil {
		return "", err
	}

	return extractTextFromPDFDocument(doc)
}

// extractTextFromPDFDocument is the shared helper that turns a pdf.Reader into a plain-text string.
// Both file- and reader-based entry points call into this function.
func extractTextFromPDFDocument(doc *pdf.Reader) (string, error) {
	if doc == nil {
		return "", errNilPDFDocument
	}

	textReader, err := doc.GetPlainText()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, textReader); err != nil {
		return "", err
	}

	return buf.String(), nil
}
