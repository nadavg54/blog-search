package filter

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Filter defines the interface for URL filtering
type Filter interface {
	ShouldKeep(ctx context.Context, url string) (bool, error)
}

// FilterURLs applies all filters to a list of URLs
func FilterURLs(ctx context.Context, urls []string, filters ...Filter) ([]string, error) {
	filtered := make([]string, 0, len(urls))

	for _, urlStr := range urls {
		keep := true
		for _, f := range filters {
			shouldKeep, err := f.ShouldKeep(ctx, urlStr)
			if err != nil {
				return nil, fmt.Errorf("filter error for URL %s: %w", urlStr, err)
			}
			if !shouldKeep {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, urlStr)
		}
	}

	return filtered, nil
}

// BaseURLFilter filters out base/root URLs
type BaseURLFilter struct{}

// NewBaseURLFilter creates a new base URL filter
func NewBaseURLFilter() *BaseURLFilter {
	return &BaseURLFilter{}
}

// ShouldKeep returns false if URL is a base/root URL
func (f *BaseURLFilter) ShouldKeep(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		// If we can't parse it, don't filter it out (let it fail later if needed)
		return true, nil
	}

	// Check if path is empty or just "/"
	path := strings.Trim(parsed.Path, "/")
	return path != "", nil
}

// AlreadyFetchedFilter filters out URLs that already exist in the provided set
type AlreadyFetchedFilter struct {
	fetchedURLs map[string]bool
}

// NewAlreadyFetchedFilter creates a new already-fetched filter
func NewAlreadyFetchedFilter(fetchedURLs map[string]bool) *AlreadyFetchedFilter {
	return &AlreadyFetchedFilter{
		fetchedURLs: fetchedURLs,
	}
}

// ShouldKeep returns false if URL is already in the fetched set
func (f *AlreadyFetchedFilter) ShouldKeep(ctx context.Context, urlStr string) (bool, error) {
	// Check if URL exists in the fetched set
	exists := f.fetchedURLs[urlStr]
	return !exists, nil
}
