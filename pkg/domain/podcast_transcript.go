package domain

import "time"

// PodcastTranscript represents a podcast episode transcript extracted from an external source
// (e.g., a PDF or TXT transcript linked from an episode page).
//
// This is intentionally separate from Article to allow transcript-specific pipelines and storage.
type PodcastTranscript struct {
	// URL is the canonical URL of the podcast episode page (not the transcript file URL).
	URL string `bson:"url" json:"url"`

	// Title is the episode title, when available.
	Title string `bson:"title" json:"title"`

	// PageContent is the extracted plain text content of the episode page that links to the transcript.
	PageContent string `bson:"page_content,omitempty" json:"page_content,omitempty"`

	// Transcript is the extracted transcript plain text.
	Transcript string `bson:"transcript" json:"transcript"`

	// TranscriptURL is the URL of the transcript file (e.g., .pdf or .txt), when available.
	TranscriptURL string `bson:"transcript_url,omitempty" json:"transcript_url,omitempty"`

	// CrawledAt is when we fetched and processed this episode/transcript.
	CrawledAt time.Time `bson:"crawled_at" json:"crawled_at"`
}
