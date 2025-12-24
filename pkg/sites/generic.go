package sites

import (
	"fmt"
	"net/url"
	"strings"

	"blog-search/pkg/urls"

	"github.com/PuerkitoBio/goquery"
)

// ExtractGenericURLs attempts to extract article URLs using common HTML patterns
// This is a fallback extractor that tries multiple strategies:
// 1. Links within <article> tags
// 2. Links within <main> content area
// 3. Links with common article-related classes (entry-title, post-title, article-link, etc.)
// 4. All links excluding navigation, footer, header, and common non-content areas
func ExtractGenericURLs(html string) ([]urls.URL, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Try to get base URL from <base> tag or use document URL
	baseURL := getBaseURL(doc)

	var result []urls.URL
	seenURLs := make(map[string]bool)

	// Strategy 1: Links within <article> tags
	doc.Find("article a").Each(func(i int, link *goquery.Selection) {
		if url := extractLink(link, baseURL, seenURLs); url != nil {
			result = append(result, *url)
		}
	})

	// Strategy 2: Links within <main> content area (if not already found)
	if len(result) == 0 {
		doc.Find("main a").Each(func(i int, link *goquery.Selection) {
			if url := extractLink(link, baseURL, seenURLs); url != nil {
				result = append(result, *url)
			}
		})
	}

	// Strategy 3: Links with common article-related classes
	articleSelectors := []string{
		"a.entry-title",
		"a.post-title",
		"a.article-link",
		"a.article-title",
		"h2 a", "h3 a", // Common pattern: title in heading with link
		".entry-title a",
		".post-title a",
		".article-title a",
	}

	for _, selector := range articleSelectors {
		doc.Find(selector).Each(func(i int, link *goquery.Selection) {
			if url := extractLink(link, baseURL, seenURLs); url != nil {
				result = append(result, *url)
			}
		})
	}

	// Strategy 4: All links excluding navigation/footer/header (last resort)
	if len(result) == 0 {
		doc.Find("body a").Not("nav a, header a, footer a, .nav a, .header a, .footer a, .menu a, .sidebar a").Each(func(i int, link *goquery.Selection) {
			if url := extractLink(link, baseURL, seenURLs); url != nil {
				// Additional filtering: skip common non-content links
				href := url.Location
				if isContentLink(href) {
					result = append(result, *url)
				}
			}
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no article URLs found in HTML using generic extractor")
	}

	return result, nil
}

// getBaseURL extracts the base URL from the HTML document
// Tries multiple sources: <base> tag, canonical link, og:url meta tag
func getBaseURL(doc *goquery.Document) string {
	// Strategy 1: Check for <base> tag
	baseTag := doc.Find("base")
	if baseTag.Length() > 0 {
		baseHref, exists := baseTag.Attr("href")
		if exists && baseHref != "" {
			return baseHref
		}
	}

	// Strategy 2: Check for canonical link
	canonical := doc.Find("link[rel='canonical']")
	if canonical.Length() > 0 {
		canonicalHref, exists := canonical.Attr("href")
		if exists && canonicalHref != "" {
			// Extract base from canonical URL (remove path)
			if parsed, err := url.Parse(canonicalHref); err == nil && parsed.IsAbs() {
				parsed.Path = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				return parsed.String()
			}
		}
	}

	// Strategy 3: Check for og:url meta tag
	ogURL := doc.Find("meta[property='og:url']")
	if ogURL.Length() > 0 {
		ogURLContent, exists := ogURL.Attr("content")
		if exists && ogURLContent != "" {
			// Extract base from og:url (remove path)
			if parsed, err := url.Parse(ogURLContent); err == nil && parsed.IsAbs() {
				parsed.Path = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				return parsed.String()
			}
		}
	}

	return ""
}

// extractLink extracts a URL from a link element if it's valid and not seen before
func extractLink(link *goquery.Selection, baseURL string, seenURLs map[string]bool) *urls.URL {
	href, exists := link.Attr("href")
	if !exists || href == "" {
		return nil
	}

	// Skip anchors, javascript, mailto, etc.
	if strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
		return nil
	}

	// Normalize URL (handle relative URLs and convert to absolute)
	normalizedHref := normalizeURL(href, baseURL)
	if normalizedHref == "" {
		return nil
	}

	// Skip if already seen
	if seenURLs[normalizedHref] {
		return nil
	}
	seenURLs[normalizedHref] = true

	// Extract title
	title := strings.TrimSpace(link.Text())
	if title == "" {
		title, _ = link.Attr("title")
		if title == "" {
			// Try parent element for title
			parent := link.Parent()
			if parent != nil {
				title = strings.TrimSpace(parent.Text())
			}
			if title == "" {
				title = normalizedHref
			}
		}
	}

	return &urls.URL{
		Location: normalizedHref,
		Title:    title,
	}
}

// normalizeURL normalizes a URL (handles relative URLs, removes fragments, converts to absolute)
func normalizeURL(href, baseURL string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}

	// Parse URL to validate and normalize
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}

	// Remove fragment
	parsed.Fragment = ""

	// If already absolute, return it
	if parsed.IsAbs() {
		return parsed.String()
	}

	// If relative URL, try to resolve it using base URL
	if baseURL != "" {
		base, err := url.Parse(baseURL)
		if err == nil {
			resolved := base.ResolveReference(parsed)
			resolved.Fragment = ""
			return resolved.String()
		}
	}

	// If no base URL and it's relative, we can't make it absolute
	// Return as-is (caller will need to handle this)
	return parsed.String()
}

// isContentLink checks if a link looks like a content/article link
func isContentLink(href string) bool {
	// Skip common non-content patterns
	skipPatterns := []string{
		"/tag/", "/category/", "/author/", "/archive/",
		"/page/", "/search", "/feed", "/rss", "/atom",
		"/login", "/register", "/about", "/contact",
		"/privacy", "/terms", "/cookie",
	}

	lowerHref := strings.ToLower(href)
	for _, pattern := range skipPatterns {
		if strings.Contains(lowerHref, pattern) {
			return false
		}
	}

	// Prefer links that look like articles (have date patterns, slugs, etc.)
	// This is a heuristic - may need adjustment
	return true
}
