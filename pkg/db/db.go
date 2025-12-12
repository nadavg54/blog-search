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
