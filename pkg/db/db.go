package db

import (
	"context"
	"fmt"

	"blog-search/pkg/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client wraps the MongoDB client and database connection
type Client struct {
	mongoClient *mongo.Client
	database    *mongo.Database
	collection  *mongo.Collection
}

// NewClient creates a new database client
func NewClient(connectionString, databaseName, collectionName string) *Client {
	clientOptions := options.Client().ApplyURI(connectionString)
	mongoClient, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		// Return client with nil - error will be caught during Connect()
		return &Client{}
	}

	database := mongoClient.Database(databaseName)
	collection := database.Collection(collectionName)

	return &Client{
		mongoClient: mongoClient,
		database:    database,
		collection:  collection,
	}
}

// Connect establishes connection to MongoDB
func (c *Client) Connect(ctx context.Context) error {
	if c.mongoClient == nil {
		return fmt.Errorf("mongo client not initialized")
	}
	return c.mongoClient.Ping(ctx, nil)
}

// Close closes the MongoDB connection
func (c *Client) Close(ctx context.Context) error {
	if c.mongoClient == nil {
		return nil
	}
	return c.mongoClient.Disconnect(ctx)
}

// SaveArticle saves an article to the database
func (c *Client) SaveArticle(ctx context.Context, article *domain.Article) error {
	if c.collection == nil {
		return fmt.Errorf("collection not initialized")
	}

	// Use URL as unique identifier for upsert operation
	filter := bson.M{"url": article.URL}
	update := bson.M{"$set": article}
	opts := options.Update().SetUpsert(true)

	_, err := c.collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (c *Client) podcastTranscriptCollection() (*mongo.Collection, error) {
	if c.database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return c.database.Collection("podcast_transcript"), nil
}

// SavePodcastTranscript saves a podcast transcript to the dedicated podcast_transcript collection.
// Uses the episode URL as a unique identifier and performs an upsert.
func (c *Client) SavePodcastTranscript(ctx context.Context, transcript *domain.PodcastTranscript) error {
	col, err := c.podcastTranscriptCollection()
	if err != nil {
		return err
	}

	filter := bson.M{"url": transcript.URL}
	update := bson.M{"$set": transcript}
	opts := options.Update().SetUpsert(true)

	_, err = col.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetExistingPodcastTranscriptURLs returns a set of URLs that already exist in the
// podcast_transcript collection for the provided list of candidate URLs.
func (c *Client) GetExistingPodcastTranscriptURLs(ctx context.Context, urls []string) (map[string]bool, error) {
	col, err := c.podcastTranscriptCollection()
	if err != nil {
		return nil, err
	}

	existing := make(map[string]bool)
	if len(urls) == 0 {
		return existing, nil
	}

	cursor, err := col.Find(
		ctx,
		bson.M{"url": bson.M{"$in": urls}},
		options.Find().SetProjection(bson.M{"url": 1, "_id": 0}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing podcast transcript URLs: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var result struct {
			URL string `bson:"url"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		if result.URL != "" {
			existing[result.URL] = true
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return existing, nil
}

// GetAllURLs fetches all URLs from the database and returns them as a map (set)
func (c *Client) GetAllURLs(ctx context.Context) (map[string]bool, error) {
	if c.collection == nil {
		return nil, fmt.Errorf("collection not initialized")
	}

	// Query to get only the URL field from all documents
	cursor, err := c.collection.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"url": 1, "_id": 0}))
	if err != nil {
		return nil, fmt.Errorf("failed to query URLs: %w", err)
	}
	defer cursor.Close(ctx)

	urlSet := make(map[string]bool)
	for cursor.Next(ctx) {
		var result struct {
			URL string `bson:"url"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue // Skip invalid documents
		}
		if result.URL != "" {
			urlSet[result.URL] = true
		}
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return urlSet, nil
}
