package domain

import "time"

// Article represents a blog article stored in the database
type Article struct {
	URL       string    `bson:"url"`
	Title     string    `bson:"title"`
	Text      string    `bson:"text"`
	CrawledAt time.Time `bson:"crawled_at"`
	// Add more fields as needed (LastMod, Priority, etc.)
}
