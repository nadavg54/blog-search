package main

import (
	"context"
	"log"
	"os"

	"blog-search/pkg/db"
	"blog-search/pkg/replication"
	"blog-search/pkg/textdownloadservice"
)

func main() {
	// Subcommand: replicate (one-shot Mongo -> Postgres)
	//
	// Example:
	//   MONGO_URI="mongodb://admin:password@localhost:27017" \
	//   POSTGRES_DSN="postgres://user:pass@localhost:5432/blogsearch?sslmode=disable" \
	//   go run . replicate
	if len(os.Args) > 1 && os.Args[1] == "replicate" {
		runReplication()
		return
	}

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

func runReplication() {
	ctx := context.Background()

	// Keep config style similar to existing Mongo usage: explicit connection strings.
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://admin:password@localhost:27017"
	}

	postgresDSN := os.Getenv("POSTGRES_DSN")

	mongo := db.NewClient(mongoURI, "blogsearch", "articles")
	if err := mongo.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		_ = mongo.Close(ctx)
	}()

	pg := db.NewPostgresClient(db.PostgresConfig{DSN: postgresDSN})
	if err := pg.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer func() {
		_ = pg.Close()
	}()

	rep, err := replication.NewReplicator(replication.Config{
		Mongo:    mongo,
		Postgres: pg,
	})
	if err != nil {
		log.Fatalf("Failed to create replicator: %v", err)
	}

	if err := rep.ReplicateArticlesMongoToPostgres(ctx); err != nil {
		log.Fatalf("Replication failed: %v", err)
	}

	log.Println("Replication done!")
}
