package seo

import (
	"context"
	"sync"
	"time"
)

const workerCount = 6

// ScanSitemapPages fetches and analyzes all sitemap page URLs concurrently.
// Uses a 6-worker pool to mirror the extension's concurrent scanning.
func ScanSitemapPages(
	ctx context.Context,
	pageURLs []string,
	homepageURL string,
	homeResponse FetchResult,
	request AuditRequest,
	rootURL string,
) []PageAnalysis {
	// Deduplicate URLs
	seen := make(map[string]bool)
	var urls []string
	for _, u := range pageURLs {
		c := ComparableURL(u)
		if !seen[c] {
			seen[c] = true
			urls = append(urls, u)
		}
	}

	results := make([]PageAnalysis, len(urls))
	var mu sync.Mutex
	indexCh := make(chan int, len(urls))
	for i := range urls {
		indexCh <- i
	}
	close(indexCh)

	homepageKey := ComparableURL(homepageURL)
	homeResponseKey := ComparableURL(homeResponse.URL)

	numWorkers := Min(workerCount, Max(1, len(urls)))
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wg.Done()
			for idx := range indexCh {
				pageURL := urls[idx]
				pageKey := ComparableURL(pageURL)

				var resp FetchResult
				if pageKey == homepageKey || pageKey == homeResponseKey {
					resp = homeResponse
				} else {
					resp = FetchText(ctx, pageURL, 15*time.Second, "GET")
				}

				var analysis PageAnalysis
				if resp.OK && resp.Text != "" {
					analysis = AnalyzeHTMLPage(pageURL, resp, request, rootURL)
				} else {
					analysis = PageAnalysis{
						OK:     false,
						Status: resp.Status,
						URL:    pageURL,
						Error:  resp.Error,
						Score:  0,
						Issues: []string{"Fetch failed or non-HTML"},
					}
				}

				mu.Lock()
				results[idx] = analysis
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}
