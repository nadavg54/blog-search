package podcasttranscriptservice

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/ledongthuc/pdf"

	"blog-search/pkg/db"
	"blog-search/pkg/domain"
)

// Service downloads podcast episode transcripts discovered via sitemaps and persists them.
// This package intentionally does not reuse existing URL discovery/content extraction pipelines yet.
type Service struct {
	db      *db.Client
	client  *http.Client
	workers int
}

// New creates a new podcast transcript service.
func New(dbClient *db.Client) *Service {
	return &Service{
		db: dbClient,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		workers: 100,
	}
}

// SetWorkers sets the number of parallel workers used to process episode URLs.
// If workers <= 0, it will be coerced to 1.
func (s *Service) SetWorkers(workers int) {
	if workers <= 0 {
		s.workers = 1
		return
	}
	s.workers = workers
}

var (
	ErrEmptySitemapURL       = errors.New("sitemap URL is empty")
	ErrEmptyEpisodeURL       = errors.New("episode URL is empty")
	ErrEmptyEpisodeHTML      = errors.New("episode HTML is empty")
	ErrNoTranscriptURL       = errors.New("no transcript URL found on episode page")
	ErrUnsupportedTranscript = errors.New("unsupported transcript type")
	ErrEmptyTranscriptText   = errors.New("extracted transcript text is empty")
)

// DownloadFromSitemap discovers episode URLs from a sitemap, then fetches each episode page,
// finds the transcript URL (PDF/TXT), downloads and extracts transcript text, and saves it.
//
// max limits the number of episode URLs processed. If max <= 0, the implementation should
// treat it as "no limit".
func (s *Service) DownloadFromSitemap(ctx context.Context, sitemapURL string, max int) error {
	if sitemapURL == "" {
		return ErrEmptySitemapURL
	}

	sitemapBytes, _, err := s.fetchURL(ctx, sitemapURL)
	if err != nil {
		return fmt.Errorf("fetch sitemap: %w", err)
	}

	episodeURLs, err := parseSitemapLocs(sitemapBytes)
	if err != nil {
		return fmt.Errorf("parse sitemap: %w", err)
	}

	if max > 0 && len(episodeURLs) > max {
		episodeURLs = episodeURLs[:max]
	}

	// Filter out episode URLs that already exist in the podcast_transcript collection.
	if s.db != nil {
		existing, err := s.db.GetExistingPodcastTranscriptURLs(ctx, episodeURLs)
		if err == nil && len(existing) > 0 {
			filtered := make([]string, 0, len(episodeURLs))
			for _, u := range episodeURLs {
				if !existing[u] {
					filtered = append(filtered, u)
				}
			}
			episodeURLs = filtered
		}
	}

	if err := s.processEpisodeURLsInParallel(ctx, episodeURLs); err != nil {
		return err
	}

	return nil
}

func (s *Service) processEpisodeURLsInParallel(ctx context.Context, episodeURLs []string) error {
	if len(episodeURLs) == 0 {
		return nil
	}

	workers := s.workers
	if workers <= 0 {
		workers = 1
	}

	jobs := make(chan string)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for episodeURL := range jobs {
				// Best-effort: skip failures for now.
				_ = s.processEpisodeURL(ctx, episodeURL)
			}
		}()
	}

	for _, episodeURL := range episodeURLs {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- episodeURL:
		}
	}

	close(jobs)
	wg.Wait()
	return nil
}

func (s *Service) processEpisodeURL(ctx context.Context, episodeURL string) error {
	episodeURL = strings.TrimSpace(episodeURL)
	if episodeURL == "" {
		return ErrEmptyEpisodeURL
	}

	episodeBytes, _, err := s.fetchURL(ctx, episodeURL)
	if err != nil {
		return fmt.Errorf("fetch episode: %w", err)
	}

	episodeHTML := string(episodeBytes)
	if strings.TrimSpace(episodeHTML) == "" {
		return ErrEmptyEpisodeHTML
	}

	title, _ := extractEpisodeTitle(episodeHTML)
	pageText, _ := extractEpisodePageText(episodeHTML)
	if strings.TrimSpace(pageText) == "" {
		// As requested: only skip if we cannot extract any meaningful page content.
		return errors.New("page content is empty")
	}

	// Transcript is best-effort: persist even if no transcript link is found.
	var (
		transcriptURL  string
		transcriptText string
	)

	if foundURL, err := findTranscriptURL(episodeHTML); err == nil && strings.TrimSpace(foundURL) != "" {
		// Resolve relative transcript URLs against the episode URL.
		if resolved, err := resolveAgainst(episodeURL, foundURL); err == nil {
			transcriptURL = resolved
			// Best-effort transcript extraction: if it fails, still persist page content.
			if text, err := s.extractTranscriptText(ctx, transcriptURL); err == nil {
				transcriptText = text
			}
		}
	}

	doc := &domain.PodcastTranscript{
		URL:           episodeURL,
		Title:         strings.TrimSpace(title),
		PageContent:   strings.TrimSpace(pageText),
		Transcript:    strings.TrimSpace(transcriptText),
		TranscriptURL: strings.TrimSpace(transcriptURL),
		CrawledAt:     time.Now(),
	}

	if s.db == nil {
		return errors.New("db client is nil")
	}

	if err := s.db.SavePodcastTranscript(ctx, doc); err != nil {
		return fmt.Errorf("save transcript: %w", err)
	}

	return nil
}

func (s *Service) fetchURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}

	// Basic "browser-like" headers to avoid 406/blocks.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	return body, resp.Header.Get("Content-Type"), nil
}

func (s *Service) extractTranscriptText(ctx context.Context, transcriptURL string) (string, error) {
	transcriptURL = strings.TrimSpace(transcriptURL)
	if transcriptURL == "" {
		return "", ErrNoTranscriptURL
	}

	body, contentType, err := s.fetchURL(ctx, transcriptURL)
	if err != nil {
		return "", err
	}

	ext := strings.ToLower(path.Ext(mustParseURLPath(transcriptURL)))
	switch ext {
	case ".txt":
		return string(body), nil
	case ".pdf":
		return extractTextFromPDFBytes(body)
	default:
		// Fallback: try by content-type.
		lct := strings.ToLower(contentType)
		switch {
		case strings.Contains(lct, "text/plain"):
			return string(body), nil
		case strings.Contains(lct, "application/pdf"):
			return extractTextFromPDFBytes(body)
		default:
			return "", ErrUnsupportedTranscript
		}
	}
}

// --- parsing helpers (private) ---

type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

type urlEntry struct {
	Location string `xml:"loc"`
}

func parseSitemapLocs(xmlBytes []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlBytes))
	var set urlSet
	if err := decoder.Decode(&set); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(set.URLs))
	for _, u := range set.URLs {
		loc := strings.TrimSpace(u.Location)
		if loc != "" {
			out = append(out, loc)
		}
	}
	return out, nil
}

func extractEpisodeTitle(html string) (string, error) {
	html = strings.TrimSpace(html)
	if html == "" {
		return "", ErrEmptyEpisodeHTML
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	if h1 := strings.TrimSpace(doc.Find("h1").First().Text()); h1 != "" {
		return h1, nil
	}
	if title := strings.TrimSpace(doc.Find("title").First().Text()); title != "" {
		return title, nil
	}
	return "", errors.New("title not found")
}

func extractEpisodePageText(html string) (string, error) {
	html = strings.TrimSpace(html)
	if html == "" {
		return "", ErrEmptyEpisodeHTML
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	// Prefer the primary post content container for this site.
	if text := strings.TrimSpace(doc.Find(".post__content").First().Text()); text != "" {
		return normalizeWhitespace(text), nil
	}

	// Fallback: whole body text.
	if text := strings.TrimSpace(doc.Find("body").First().Text()); text != "" {
		return normalizeWhitespace(text), nil
	}

	return "", errors.New("page content not found")
}

func normalizeWhitespace(s string) string {
	// Collapse runs of whitespace into single spaces for a compact searchable string.
	return strings.Join(strings.Fields(s), " ")
}

func findTranscriptURL(html string) (string, error) {
	html = strings.TrimSpace(html)
	if html == "" {
		return "", ErrEmptyEpisodeHTML
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	type candidate struct {
		href string
		text string
	}

	var (
		high []candidate
		med  []candidate
		low  []candidate
	)

	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		text := strings.TrimSpace(sel.Text())
		docLike := isTranscriptDocumentHref(href)
		textLike := strings.Contains(strings.ToLower(text), "transcript")

		c := candidate{href: href, text: text}
		switch {
		case docLike && textLike:
			high = append(high, c)
		case docLike:
			med = append(med, c)
		case textLike:
			low = append(low, c)
		}
	})

	switch {
	case len(high) > 0:
		return high[0].href, nil
	case len(med) > 0:
		return med[0].href, nil
	case len(low) > 0:
		return low[0].href, nil
	default:
		return "", ErrNoTranscriptURL
	}
}

func isTranscriptDocumentHref(href string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return hasTranscriptExt(href)
	}
	return hasTranscriptExt(u.Path)
}

func hasTranscriptExt(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".pdf", ".txt":
		return true
	default:
		return false
	}
}

func resolveAgainst(baseURL, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ErrNoTranscriptURL
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(u).String(), nil
}

func mustParseURLPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func extractTextFromPDFBytes(pdfBytes []byte) (string, error) {
	if len(pdfBytes) == 0 {
		return "", errors.New("empty pdf bytes")
	}

	r := bytes.NewReader(pdfBytes)
	doc, err := pdf.NewReader(r, int64(len(pdfBytes)))
	if err != nil {
		return "", err
	}

	textReader, err := doc.GetPlainText()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, textReader); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}
