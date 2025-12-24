package urls

import (
	"context"
	"net/url"
	"strings"
)

// UrlFilter defines the interface for URL filtering
type UrlFilter interface {
	ShouldKeep(ctx context.Context, url string) (bool, error)
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

// ContainsPathFilter filters URLs to only keep those that contain a specific path segment
type ContainsPathFilter struct {
	pathSegment string // The path segment to check for (e.g., "/blog")
}

// NewContainsPathFilter creates a new path filter that keeps URLs containing the specified path segment
func NewContainsPathFilter(pathSegment string) *ContainsPathFilter {
	return &ContainsPathFilter{
		pathSegment: pathSegment,
	}
}

// ShouldKeep returns true if URL contains the specified path segment
func (f *ContainsPathFilter) ShouldKeep(ctx context.Context, urlStr string) (bool, error) {
	return strings.Contains(urlStr, f.pathSegment), nil
}
