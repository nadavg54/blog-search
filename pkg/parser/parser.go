package parser

// URL represents a URL entry from a parser (sitemap or RSS)
type URL struct {
	Location string // URL of the article
	Title    string // Title of the article (optional)
	// Add more fields as needed (LastMod, PublishDate, etc.)
}

// Parser defines the interface for URL parsers (sitemap, RSS, etc.)
type Parser interface {
	ParseFromURL(url string) ([]URL, error)
}
