package sites

import (
	"fmt"
	"strings"

	"blog-search/pkg/urls"
	"github.com/PuerkitoBio/goquery"
)

// ExtractSERadioURLs extracts article URLs from se-radio.net HTML pages
// It looks for articles in the div with class "col-12 megaphone-order-1 col-lg-8"
// and extracts links from h2.entry-title > a elements
func ExtractSERadioURLs(html string) ([]urls.URL, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	var result []urls.URL

	// Find the container div with class "col-12 megaphone-order-1 col-lg-8"
	doc.Find("div.col-12.megaphone-order-1.col-lg-8").Each(func(i int, container *goquery.Selection) {
		// Find all article elements within the container
		container.Find("article.megaphone-item.megaphone-post").Each(func(j int, article *goquery.Selection) {
			// Find the h2.entry-title > a link
			link := article.Find("h2.entry-title a")
			if link.Length() == 0 {
				return
			}

			href, exists := link.Attr("href")
			if !exists || href == "" {
				return
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
	})

	if len(result) == 0 {
		return nil, fmt.Errorf("no article URLs found in HTML")
	}

	return result, nil
}

