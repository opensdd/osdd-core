package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ValidateURL checks that rawURL is non-empty and uses an http or https scheme.
func ValidateURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("url cannot be empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q for url %s", parsed.Scheme, rawURL)
	}
	return nil
}

// FetchURL performs a single HTTP GET request to rawURL and returns the
// response body as raw bytes. It validates that rawURL is non-empty and
// uses an http or https scheme. Non-2xx status codes are treated as errors.
// Retry logic is NOT handled here — callers are expected to retry as needed.
func FetchURL(ctx context.Context, rawURL string) ([]byte, error) {
	if err := ValidateURL(rawURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", rawURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("url fetch %s returned status %d", rawURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", rawURL, err)
	}

	return body, nil
}
