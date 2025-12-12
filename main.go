package main

import (
	"fmt"
	"log"
	"os"

	"blog-search/pkg/sitemap"
)

func main() {
	// For now, hardcode the sitemap URL
	sitemapURL := "https://developers.googleblog.com/sitemap.xml"

	if len(os.Args) > 1 {
		sitemapURL = os.Args[1]
	}

	parser := sitemap.NewParser()

	entries, err := parser.ParseFromURL(sitemapURL)
	if err != nil {
		log.Fatalf("Failed to parse sitemap: %v", err)
	}

	// Print first 10 entries
	maxEntries := 10
	if len(entries) < maxEntries {
		maxEntries = len(entries)
	}

	fmt.Printf("Found %d entries. Showing first %d:\n\n", len(entries), maxEntries)

	for i := 0; i < maxEntries; i++ {
		entry := entries[i]
		fmt.Printf("Entry %d:\n", i+1)
		fmt.Printf("  URL: %s\n", entry.Location)
		if entry.LastMod != "" {
			fmt.Printf("  Last Modified: %s\n", entry.LastMod)
		}
		if entry.Priority != "" {
			fmt.Printf("  Priority: %s\n", entry.Priority)
		}
		fmt.Println()
	}
}
