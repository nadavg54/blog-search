package content

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	errEmptyHTML         = errors.New("empty HTML content")
	errNoTranscriptLink  = errors.New("no transcript link found in HTML")
	errFailedToParseHTML = errors.New("failed to parse HTML for transcript link")
)

// FindTranscriptURL attempts to locate a transcript link (PDF, TXT, etc.) in the
// HTML content of a podcast episode page.
//
// The current strategy is:
//   - Parse the HTML with goquery
//   - Collect all <a> elements with an href
//   - Rank them by how much they look like a transcript link:
//     1) Anchor text mentions "transcript" and href looks like a document (.pdf/.txt)
//     2) href looks like a document (.pdf/.txt)
//     3) Anchor text mentions "transcript"
//   - Return the best-matching href, or an error if none are found.
//
// The caller is responsible for resolving relative URLs against any base URL, if needed.
func FindTranscriptURL(html string) (string, error) {
	html = strings.TrimSpace(html)
	if html == "" {
		return "", errEmptyHTML
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", errors.Join(errFailedToParseHTML, err)
	}

	type candidate struct {
		href string
		text string
	}

	var (
		highPriority   []candidate // text mentions transcript AND href is document-like
		mediumPriority []candidate // href is document-like
		lowPriority    []candidate // text mentions transcript
	)

	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists {
			return
		}

		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		text := strings.TrimSpace(sel.Text())

		docLike := isDocumentLikeTranscriptHref(href)
		textMentionsTranscript := anchorTextMentionsTranscript(text)

		c := candidate{href: href, text: text}

		switch {
		case docLike && textMentionsTranscript:
			highPriority = append(highPriority, c)
		case docLike:
			mediumPriority = append(mediumPriority, c)
		case textMentionsTranscript:
			lowPriority = append(lowPriority, c)
		}
	})

	if len(highPriority) > 0 {
		return highPriority[0].href, nil
	}
	if len(mediumPriority) > 0 {
		return mediumPriority[0].href, nil
	}
	if len(lowPriority) > 0 {
		return lowPriority[0].href, nil
	}

	return "", errNoTranscriptLink
}

// isDocumentLikeTranscriptHref returns true if the href looks like a transcript
// document we should try to fetch (e.g., .pdf or .txt).
func isDocumentLikeTranscriptHref(href string) bool {
	parsed, err := url.Parse(href)
	if err != nil {
		// If the URL can't be parsed, fall back to a simple suffix check
		return hasTranscriptFileExtension(href)
	}

	return hasTranscriptFileExtension(parsed.Path)
}

func hasTranscriptFileExtension(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case ".pdf", ".txt":
		return true
	default:
		return false
	}
}

// anchorTextMentionsTranscript returns true if the anchor text clearly refers to
// a transcript link.
func anchorTextMentionsTranscript(text string) bool {
	if text == "" {
		return false
	}

	lower := strings.ToLower(text)
	return strings.Contains(lower, "transcript")
}
