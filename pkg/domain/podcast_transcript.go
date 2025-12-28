package domain

import "time"

// PodcastTranscript represents a podcast transcript document from MongoDB
type PodcastTranscript struct {
	URL         string    `bson:"url"`
	Title       string    `bson:"title"`
	Transcript  string    `bson:"transcript"`
	PageContent string    `bson:"page_content"`
	CrawledAt   time.Time `bson:"crawled_at"`
}

