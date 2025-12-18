package main

import (
	"context"
	"flag"
	"log"
	"time"

	"blog-search/pkg/db"
	"blog-search/pkg/podcasttranscriptservice"
)

func main() {
	var (
		sitemapURL = flag.String("sitemap", "https://softwareengineeringdaily.com/image-sitemap-2.xml", "Sitemap URL to crawl for episode URLs")
		max        = flag.Int("max", 100, "Max episode URLs to process (<=0 means no limit)")
		workers    = flag.Int("workers", 100, "Number of parallel workers to process episode URLs")

		mongoURI   = flag.String("mongo-uri", "mongodb://admin:password@localhost:27017", "MongoDB connection string")
		dbName     = flag.String("db", "blogsearch", "MongoDB database name")
		collection = flag.String("collection", "articles", "Default collection used by db.Client for articles (not used by podcast transcript flow)")
	)
	flag.Parse()

	ctx := context.Background()

	dbClient := db.NewClient(*mongoURI, *dbName, *collection)
	if err := dbClient.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close(ctx)

	service := podcasttranscriptservice.New(dbClient)
	service.SetWorkers(*workers)

	start := time.Now()
	log.Printf("Processing podcast transcripts from sitemap: %s (max=%d)", *sitemapURL, *max)
	if err := service.DownloadFromSitemap(ctx, *sitemapURL, *max); err != nil {
		log.Fatalf("Podcast transcript download failed: %v", err)
	}
	log.Printf("Done. Duration: %s", time.Since(start))
}
