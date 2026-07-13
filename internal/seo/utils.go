// Package seo contains all business logic for the SEO audit engine.
// Logic ported from the Voxlumedia AI SEO Audit browser extension (report.js v1.0.7).
package seo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// ─────────────────────────────────────────────────────────────────────────────
// URL helpers
// ─────────────────────────────────────────────────────────────────────────────

// NormalizeURL ensures the URL has an https:// scheme and no fragment.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty URL")
	}
	if !regexp.MustCompile(`(?i)^https?://`).MatchString(raw) {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	u.Fragment = ""
	return u.String(), nil
}

// DomainFromURL returns the bare hostname without "www." prefix.
func DomainFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "website"
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// SlugFromDomain converts a domain to a URL-safe slug.
func SlugFromDomain(domain string) string {
	domain = DomainFromURL(domain)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(strings.ToLower(domain), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "seo-report"
	}
	return slug
}

// AbsoluteURL resolves href relative to base. Returns "" on failure.
func AbsoluteURL(href, base string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	h, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return b.ResolveReference(h).String()
}

// ComparableURL strips fragment and trailing slash (except root) for dedup.
func ComparableURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	u.RawQuery = ""
	out := u.String()
	if u.Path != "/" && strings.HasSuffix(out, "/") {
		out = out[:len(out)-1]
	}
	return out
}

// SameRootHost returns true when a and b share the same hostname (ignoring www).
func SameRootHost(a, b string) bool {
	ua, err1 := url.Parse(a)
	ub, err2 := url.Parse(b)
	if err1 != nil || err2 != nil {
		return false
	}
	ha := strings.ToLower(strings.TrimPrefix(ua.Hostname(), "www."))
	hb := strings.ToLower(strings.TrimPrefix(ub.Hostname(), "www."))
	return ha == hb
}

// IsLikelyHTMLPageURL returns true when the URL path doesn't look like a binary asset.
func IsLikelyHTMLPageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	path := strings.ToLower(u.Path)
	assetExt := regexp.MustCompile(`\.(jpg|jpeg|png|gif|webp|svg|ico|pdf|doc|docx|xls|xlsx|ppt|pptx|zip|rar|7z|mp4|mov|avi|mp3|wav|json|xml|txt|css|js|woff|woff2|ttf|eot)(\?|$)`)
	return !assetExt.MatchString(path)
}

// ─────────────────────────────────────────────────────────────────────────────
// Text helpers
// ─────────────────────────────────────────────────────────────────────────────

// WordCount returns the number of words in a text string.
func WordCount(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-' {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}

// NormalizePhone strips all non-digit characters from a phone string.
func NormalizePhone(phone string) string {
	re := regexp.MustCompile(`\D+`)
	return re.ReplaceAllString(phone, "")
}

// ContainsAny returns true if text contains any of the given words (case-insensitive).
func ContainsAny(text string, words []string) bool {
	lower := strings.ToLower(text)
	for _, w := range words {
		if strings.Contains(lower, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Scoring helpers
// ─────────────────────────────────────────────────────────────────────────────

// StatusWeight converts a check status to a numeric weight for averaging.
// Ported directly from extension: pass=100, warn=62, fail=18, info/other=75
func StatusWeight(status string) int {
	switch status {
	case "pass":
		return 100
	case "warn":
		return 62
	case "fail":
		return 18
	default:
		return 75
	}
}

// Grade converts a numeric score to a letter grade.
func Grade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// ImpactRank returns a numeric rank for sorting by impact level.
func ImpactRank(impact string) int {
	switch strings.ToLower(impact) {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

// Percent safely computes round(value/total * 100).
func Percent(value, total int) int {
	if total == 0 {
		return 0
	}
	return int(float64(value) / float64(total) * 100)
}

// Min returns the smaller of two ints.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two ints.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// String comparison helpers (for local listing NAP match)
// ─────────────────────────────────────────────────────────────────────────────

func CompareText(a, b string) string {
	norm := func(s string) string {
		re := regexp.MustCompile(`[^a-z0-9]+`)
		return strings.TrimSpace(re.ReplaceAllString(strings.ToLower(s), " "))
	}
	aa, bb := norm(a), norm(b)
	if aa == "" || bb == "" {
		return "Not Provided"
	}
	if aa == bb || strings.Contains(aa, bb) || strings.Contains(bb, aa) {
		return "Match"
	}
	return "Review"
}

func ComparePhone(a, b string) string {
	aa, bb := NormalizePhone(a), NormalizePhone(b)
	if aa == "" || bb == "" {
		return "Not Provided"
	}
	n := func(s string) string {
		if len(s) >= 10 {
			return s[len(s)-10:]
		}
		return s
	}
	if n(aa) == n(bb) {
		return "Match"
	}
	return "Review"
}

func CompareDomain(a, b string) string {
	da := DomainFromURL(a)
	db := DomainFromURL(b)
	if da == "" || db == "" {
		return "Not Provided"
	}
	if da == db || strings.Contains(da, db) || strings.Contains(db, da) {
		return "Match"
	}
	return "Review"
}
