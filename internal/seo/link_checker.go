package seo

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BrokenLinksResult holds the outcome of the broken link scan.
type BrokenLinksResult struct {
	Checked        int
	TotalDiscovered int
	Broken         int
	Rows           []BrokenLinkRow
	Skipped        string
}

// BrokenLinkRow represents a single broken/blocked link.
type BrokenLinkRow struct {
	URL    string
	Source string
	Status string
	Anchor string
}

// CheckBrokenLinks scans up to `limit` unique links from all page analyses.
// Uses a 5-worker pool with HEAD→GET fallback, mirroring the extension.
func CheckBrokenLinks(ctx context.Context, pageAnalyses []PageAnalysis, limit int) BrokenLinksResult {
	if limit <= 0 {
		return BrokenLinksResult{Skipped: "Disabled in settings."}
	}

	// Collect unique links from all successfully scanned pages
	type linkEntry struct {
		URL    string
		Anchor string
		Source string
	}
	seen := make(map[string]bool)
	var allLinks []linkEntry
	for _, p := range pageAnalyses {
		if !p.OK {
			continue
		}
		for _, l := range p.Links {
			if l.Href == "" || seen[l.Href] {
				continue
			}
			seen[l.Href] = true
			allLinks = append(allLinks, linkEntry{URL: l.Href, Anchor: l.Text, Source: p.URL})
		}
	}

	totalDiscovered := len(allLinks)
	sample := allLinks
	if len(sample) > limit {
		sample = sample[:limit]
	}

	// 5-worker concurrent broken link check
	type result struct {
		entry  linkEntry
		status int
		ok     bool
	}
	results := make([]result, len(sample))
	indexCh := make(chan int, len(sample))
	for i := range sample {
		indexCh <- i
	}
	close(indexCh)

	numWorkers := Min(5, Max(1, len(sample)))
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wg.Done()
			for idx := range indexCh {
				entry := sample[idx]
				// Try HEAD first
				res := FetchText(ctx, entry.URL, 9*time.Second, "HEAD")
				// Fallback to GET for 0, 405, 403
				if !res.OK && (res.Status == 0 || res.Status == 405 || res.Status == 403) {
					res = FetchText(ctx, entry.URL, 9*time.Second, "GET")
				}
				mu.Lock()
				results[idx] = result{entry: entry, status: res.Status, ok: res.OK}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Collect broken rows
	var rows []BrokenLinkRow
	for _, r := range results {
		if !r.ok || r.status >= 400 {
			statusStr := fmt.Sprintf("%d", r.status)
			if r.status == 0 {
				statusStr = "blocked"
			}
			rows = append(rows, BrokenLinkRow{
				URL:    r.entry.URL,
				Source: r.entry.Source,
				Status: statusStr,
				Anchor: r.entry.Anchor,
			})
		}
	}

	// Cap displayed rows at 30
	if len(rows) > 30 {
		rows = rows[:30]
	}

	return BrokenLinksResult{
		Checked:         len(sample),
		TotalDiscovered: totalDiscovered,
		Broken:          len(rows),
		Rows:            rows,
	}
}
