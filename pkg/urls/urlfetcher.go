package urls

// URL represents a URL entry from a parser (sitemap or RSS)
type URL struct {
	Location string // URL of the article
	Title    string // Title of the article (optional)
	// Add more fields as needed (LastMod, PublishDate, etc.)
}

// URLsFetcher defines the interface for URL parsers (sitemap, RSS, etc.)
type URLsFetcher interface {
	Fetch(baseUrl string) ([]URL, error)
}
