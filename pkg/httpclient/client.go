package httpclient

import (
	"net/http"
)

// ClientType represents the type of HTTP client configuration
type ClientType string

const (
	// BrowserClient uses browser-like headers to avoid 406 (Not Acceptable) errors
	// Used for sites that require browser-like User-Agent and headers
	BrowserClient ClientType = "browser"

	// CloudflareClient uses simple headers (like curl) to avoid 403 (Forbidden) errors
	// Used for Cloudflare-protected sites that block browser-like User-Agents
	CloudflareClient ClientType = "cloudflare"
)

// HTTPClient wraps an http.Client with configuration
type HTTPClient struct {
	client    *http.Client
	clientType ClientType
}

// NewClient creates a new HTTP client with the specified type
func NewClient(clientType ClientType) *HTTPClient {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow up to 10 redirects
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return &HTTPClient{
		client:     client,
		clientType: clientType,
	}
}

// Do executes an HTTP request with the appropriate headers for the client type
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.setHeaders(req)
	return c.client.Do(req)
}

// Get is a convenience method for GET requests
func (c *HTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// setHeaders sets the appropriate headers based on client type
func (c *HTTPClient) setHeaders(req *http.Request) {
	switch c.clientType {
	case BrowserClient:
		// Browser-like headers to avoid 406 (Not Acceptable) errors
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

	case CloudflareClient:
		// Simple headers like curl to avoid 403 (Forbidden) errors from Cloudflare
		// Cloudflare allows simple tools like curl but blocks browser-like User-Agents
		req.Header.Set("User-Agent", "curl/8.7.1")

	default:
		// Default: use Go's default User-Agent
	}
}

