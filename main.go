package main

import (
	"context"
	"log"
	"os"

	"blog-search/pkg/db"
	"blog-search/pkg/textdownloadservice"
)

func main() {
	// Get sitemap URL from command line or use default
	sitemapURL := "https://blog.cloudflare.com/sitemap-posts.xml"

	if len(os.Args) > 1 {
		sitemapURL = os.Args[1]
	}

	// Initialize database client
	dbClient := db.NewClient("mongodb://admin:password@localhost:27017", "blogsearch", "articles")
	ctx := context.Background()

	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	// Create service
	service := textdownloadservice.NewService(textdownloadservice.Config{
		DBClient:    dbClient,
		WorkerCount: 50,
		MaxEntries:  10000,
	})

	// Download articles from sitemap using the service
	log.Printf("Processing articles from sitemap: %s", sitemapURL)
	if err := service.DownloadText(ctx, sitemapURL, 10000); err != nil {
		log.Fatalf("Failed to download articles: %v", err)
	}

	log.Println("All done!")
}
