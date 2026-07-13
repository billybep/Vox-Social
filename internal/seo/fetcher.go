package seo

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// FetchResult holds the outcome of a single HTTP fetch.
type FetchResult struct {
	OK          bool
	Status      int
	URL         string
	Text        string
	Length      int
	ContentType string
	Headers     map[string]string
	Error       string
}

// httpClient is shared across all fetches. No global timeout — context controls it.
var httpClient = &http.Client{
	Timeout: 0,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// FetchText performs an HTTP GET (or HEAD) and returns a FetchResult.
// timeout is the per-request context deadline in addition to the caller's ctx.
func FetchText(ctx context.Context, rawURL string, timeout time.Duration, method string) FetchResult {
	if method == "" {
		method = http.MethodGet
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, nil)
	if err != nil {
		log.Printf("[SEO API] ERROR creating request %s %s: %v", method, rawURL, err)
		return FetchResult{OK: false, URL: rawURL, Error: err.Error()}
	}
	

	// Mimic a real browser to avoid bot blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[SEO API] ERROR %s %s: %v", method, rawURL, err)
		return FetchResult{OK: false, Status: 0, URL: rawURL, Error: err.Error()}
	}

	defer resp.Body.Close()

	headers := headersToMap(resp.Header)
	finalURL := resp.Request.URL.String()
	if finalURL == "" {
		finalURL = rawURL
	}

	var text string
	var length int
	if method != http.MethodHead {
		// Read up to 3 MB to prevent memory bloat on huge pages
		lr := io.LimitReader(resp.Body, 3*1024*1024)
		b, readErr := io.ReadAll(lr)
		if readErr == nil {
			text = string(b)
			length = len(b)
		}
	}

	return FetchResult{
		OK:          resp.StatusCode >= 200 && resp.StatusCode < 400,
		Status:      resp.StatusCode,
		URL:         finalURL,
		Text:        text,
		Length:      length,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     headers,
	}
}

// FetchJSON performs an HTTP request and returns a FetchResult.
// The caller is responsible for JSON parsing of result.Text.
func FetchJSON(ctx context.Context, rawURL string, opts *http.Request, timeout time.Duration) FetchResult {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var req *http.Request
	var err error
	if opts != nil {
		req = opts.WithContext(reqCtx)
	} else {
		req, err = http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			log.Printf("[SEO API] ERROR creating request JSON %s: %v", rawURL, err)
			return FetchResult{OK: false, URL: rawURL, Error: err.Error()}
		}
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}


	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[SEO API] ERROR JSON %s %s: %v", method, rawURL, err)
		return FetchResult{OK: false, URL: rawURL, Error: err.Error()}
	}

	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	headers := headersToMap(resp.Header)
	finalURL := resp.Request.URL.String()

	return FetchResult{
		OK:          resp.StatusCode >= 200 && resp.StatusCode < 400,
		Status:      resp.StatusCode,
		URL:         finalURL,
		Text:        string(b),
		Length:      len(b),
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     headers,
	}
}

func headersToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, vv := range h {
		m[strings.ToLower(k)] = strings.Join(vv, ", ")
	}
	return m
}
