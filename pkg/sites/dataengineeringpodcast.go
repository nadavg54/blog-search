package sites

import (
	"fmt"
	"strings"

	"blog-search/pkg/urls"
	"github.com/PuerkitoBio/goquery"
)

// ExtractDataEngineeringPodcastURLs extracts episode URLs from dataengineeringpodcast.com HTML pages
// It looks for links with class "episodeLink" that have href starting with "/episodepage/"
func ExtractDataEngineeringPodcastURLs(html string) ([]urls.URL, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var result []urls.URL

	// Find all episode links
	doc.Find("a.episodeLink").Each(func(i int, link *goquery.Selection) {
		href, exists := link.Attr("href")
		if !exists || href == "" {
			return
		}

		// Make sure it's an episode link
		if !strings.HasPrefix(href, "/episodepage/") {
			return
		}

		// Convert relative URL to absolute
		if strings.HasPrefix(href, "/") {
			href = "https://www.dataengineeringpodcast.com" + href
		}

		title := strings.TrimSpace(link.Text())
		if title == "" {
			// Fallback: try to get title from the link's title attribute or use href
			title, _ = link.Attr("title")
			if title == "" {
				title = href
			}
		}

		result = append(result, urls.URL{
			Location: href,
			Title:    title,
		})
	})

	if len(result) == 0 {
		return nil, fmt.Errorf("no episode URLs found in HTML")
	}

	return result, nil
}


