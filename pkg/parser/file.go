package parser

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// FileParser handles reading URLs from a file (one URL per line)
type FileParser struct{}

// NewFileParser creates a new file parser
func NewFileParser() *FileParser {
	return &FileParser{}
}

// ParseFromURL reads URLs from a file (file path is passed as the "url" parameter)
func (p *FileParser) ParseFromURL(filePath string) ([]URL, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var urls []URL
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Remove trailing commas and whitespace
		line = strings.TrimRight(line, ", \t")

		if line == "" {
			continue
		}

		// Add URL (title will be empty, can be extracted later if needed)
		urls = append(urls, URL{
			Location: line,
			Title:    "", // Title not available from file
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file at line %d: %w", lineNum, err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs found in file")
	}

	return urls, nil
}
