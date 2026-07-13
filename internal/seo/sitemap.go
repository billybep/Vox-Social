package seo

import (
	"context"
	"encoding/xml"
	"regexp"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

type SitemapResult struct {
	URLs     []string
	Sitemaps []string
}

type SitemapDiscovery struct {
	SitemapURLs []string
	PageURLs    []string
}

// ─────────────────────────────────────────────────────────────────────────────
// XML parsing
// ─────────────────────────────────────────────────────────────────────────────

// sitemapIndex and urlset for XML parsing
type sitemapIndex struct {
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

type urlSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
}

// ParseSitemapXML parses an XML sitemap/sitemap-index and returns discovered URLs.
func ParseSitemapXML(text string) SitemapResult {
	result := SitemapResult{}
	if text == "" {
		return result
	}

	// Try as sitemap index
	var idx sitemapIndex
	if err := xml.Unmarshal([]byte(text), &idx); err == nil && len(idx.Sitemaps) > 0 {
		for _, s := range idx.Sitemaps {
			if loc := strings.TrimSpace(s.Loc); loc != "" {
				result.Sitemaps = append(result.Sitemaps, loc)
			}
		}
	}

	// Try as urlset
	var us urlSet
	if err := xml.Unmarshal([]byte(text), &us); err == nil && len(us.URLs) > 0 {
		for _, u := range us.URLs {
			if loc := strings.TrimSpace(u.Loc); loc != "" {
				result.URLs = append(result.URLs, loc)
			}
		}
	}

	// Regex fallback for malformed XML
	if len(result.URLs) == 0 && len(result.Sitemaps) == 0 {
		re := regexp.MustCompile(`(?i)<loc>\s*([^<]+)\s*</loc>`)
		matches := re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			loc := strings.TrimSpace(m[1])
			if regexp.MustCompile(`(?i)\.xml(\.gz)?($|\?)`).MatchString(loc) {
				result.Sitemaps = append(result.Sitemaps, loc)
			} else {
				result.URLs = append(result.URLs, loc)
			}
		}
	}

	result.URLs = unique(result.URLs)
	result.Sitemaps = unique(result.Sitemaps)
	return result
}

// ExtractRobotSitemaps parses "Sitemap: https://..." lines from robots.txt.
func ExtractRobotSitemaps(robotsText string) []string {
	re := regexp.MustCompile(`(?im)^\s*Sitemap:\s*(\S+)\s*$`)
	matches := re.FindAllStringSubmatch(robotsText, -1)
	var out []string
	for _, m := range matches {
		if loc := strings.TrimSpace(m[1]); loc != "" {
			out = append(out, loc)
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Discovery
// ─────────────────────────────────────────────────────────────────────────────

// DiscoverSitemapPages fetches all XML sitemaps and returns discovered page URLs.
// Mirrors the extension's discoverSitemapPages() function.
func DiscoverSitemapPages(ctx context.Context, origin, robotsText string, commonSitemapResults []FetchResult) SitemapDiscovery {
	seenSitemaps := make(map[string]bool)
	pageURLs := make(map[string]bool)
	sitemapURLs := make(map[string]bool)

	// Build initial queue
	candidates := []string{
		origin + "/sitemap.xml",
		origin + "/sitemap_index.xml",
		origin + "/wp-sitemap.xml",
		origin + "/page-sitemap.xml",
		origin + "/post-sitemap.xml",
	}
	// Add from robots.txt
	for _, u := range ExtractRobotSitemaps(robotsText) {
		abs := AbsoluteURL(u, origin)
		if abs != "" {
			candidates = append(candidates, abs)
		}
	}

	// Pre-process already-fetched common sitemaps
	for _, resp := range commonSitemapResults {
		if resp.OK && (strings.Contains(resp.Text, "<urlset") || strings.Contains(resp.Text, "<sitemapindex") || strings.Contains(resp.Text, "<loc>")) {
			parsed := ParseSitemapXML(resp.Text)
			sitemapURLs[resp.URL] = true
			for _, u := range parsed.URLs {
				pageURLs[u] = true
			}
			for _, s := range parsed.Sitemaps {
				abs := AbsoluteURL(s, origin)
				if abs != "" {
					candidates = append(candidates, abs)
				}
			}
		}
	}

	// Remove dupes from candidate list
	queue := unique(candidates)

	// BFS fetch sitemaps
	for len(queue) > 0 {
		sitemapURL := queue[0]
		queue = queue[1:]

		if seenSitemaps[sitemapURL] {
			continue
		}
		seenSitemaps[sitemapURL] = true

		// Reuse pre-fetched result if available
		var resp *FetchResult
		for i := range commonSitemapResults {
			if ComparableURL(commonSitemapResults[i].URL) == ComparableURL(sitemapURL) {
				resp = &commonSitemapResults[i]
				break
			}
		}

		var fetched FetchResult
		if resp == nil {
			fetched = FetchText(ctx, sitemapURL, 15*time.Second, "GET")
			resp = &fetched
		}

		if !resp.OK || resp.Text == "" {
			continue
		}

		sitemapURLs[resp.URL] = true
		parsed := ParseSitemapXML(resp.Text)
		for _, u := range parsed.URLs {
			pageURLs[u] = true
		}
		for _, s := range parsed.Sitemaps {
			abs := AbsoluteURL(s, origin)
			if abs != "" && !seenSitemaps[abs] {
				queue = append(queue, abs)
			}
		}
	}

	// Filter page URLs to same-host HTML-likely URLs
	var filteredPages []string
	for u := range pageURLs {
		if SameRootHost(u, origin) && IsLikelyHTMLPageURL(u) {
			filteredPages = append(filteredPages, u)
		}
	}

	var filteredSitemaps []string
	for u := range sitemapURLs {
		filteredSitemaps = append(filteredSitemaps, u)
	}

	return SitemapDiscovery{
		SitemapURLs: filteredSitemaps,
		PageURLs:    filteredPages,
	}
}

// unique deduplicates a string slice while preserving order.
func unique(items []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
