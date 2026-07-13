package seo

import (
	"regexp"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Detector types
// ─────────────────────────────────────────────────────────────────────────────

type SecurityHeaders struct {
	HSTS              bool
	CSP               bool
	XFrameOptions     bool
	XContentType      bool
	ReferrerPolicy    bool
	PermissionsPolicy bool
	Server            string
	PoweredBy         string
}

type SuspiciousResult struct {
	Risk    string // low | medium | high
	Details string
}

// ─────────────────────────────────────────────────────────────────────────────
// Security headers
// ─────────────────────────────────────────────────────────────────────────────

// AnalyzeSecurityHeaders inspects the response header map and returns a struct.
func AnalyzeSecurityHeaders(headers map[string]string) SecurityHeaders {
	return SecurityHeaders{
		HSTS:              headers["strict-transport-security"] != "",
		CSP:               headers["content-security-policy"] != "",
		XFrameOptions:     headers["x-frame-options"] != "" || headers["content-security-policy"] != "",
		XContentType:      headers["x-content-type-options"] != "",
		ReferrerPolicy:    headers["referrer-policy"] != "",
		PermissionsPolicy: headers["permissions-policy"] != "",
		Server:            headers["server"],
		PoweredBy:         headers["x-powered-by"],
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Schema type detection
// ─────────────────────────────────────────────────────────────────────────────

// DetectSchemaTypes finds JSON-LD @type values and known microdata itemtype patterns.
func DetectSchemaTypes(schemaText string) []string {
	found := make(map[string]bool)

	// JSON-LD @type
	re := regexp.MustCompile(`"@type"\s*:\s*"([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(schemaText, -1) {
		found[m[1]] = true
	}

	// Known common types via keyword scan
	known := []string{
		"LocalBusiness", "Organization", "WebSite", "BreadcrumbList", "FAQPage",
		"Article", "Service", "Product", "Review", "ProfessionalService",
		"HomeAndConstructionBusiness", "Restaurant", "Store",
	}
	for _, t := range known {
		if regexp.MustCompile(`(?i)` + regexp.QuoteMeta(t)).MatchString(schemaText) {
			found[t] = true
		}
	}

	result := make([]string, 0, len(found))
	for k := range found {
		result = append(result, k)
		if len(result) >= 12 {
			break
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Technology detection
// ─────────────────────────────────────────────────────────────────────────────

// DetectTech identifies common CMS/framework signatures from HTML source and generator meta.
func DetectTech(html, generator string) []string {
	source := strings.ToLower(html + "\n" + generator)
	type techPattern struct {
		name    string
		pattern string
	}
	techs := []techPattern{
		{"WordPress", `wp-content|wp-includes|wordpress`},
		{"Elementor", `elementor`},
		{"WooCommerce", `woocommerce`},
		{"Shopify", `cdn\.shopify|shopify`},
		{"Wix", `wixstatic|wix\.com`},
		{"Squarespace", `squarespace`},
		{"Webflow", `webflow`},
		{"React", `react`},
		{"Next.js", `__next`},
		{"jQuery", `jquery`},
		{"Bootstrap", `bootstrap`},
		{"GTM", `googletagmanager`},
		{"GA4/Google Analytics", `gtag|google-analytics`},
	}

	seen := make(map[string]bool)
	var result []string
	for _, t := range techs {
		if regexp.MustCompile(t.pattern).MatchString(source) && !seen[t.name] {
			seen[t.name] = true
			result = append(result, t.name)
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Social profile detection
// ─────────────────────────────────────────────────────────────────────────────

type Link struct {
	Href string
	Text string
}

// DetectSocialProfiles finds common social media platform links in the page's link list.
func DetectSocialProfiles(links []Link) []string {
	platforms := map[string]string{
		"Facebook":  `facebook\.com`,
		"Instagram": `instagram\.com`,
		"LinkedIn":  `linkedin\.com`,
		"YouTube":   `youtube\.com|youtu\.be`,
		"TikTok":    `tiktok\.com`,
		"X/Twitter": `twitter\.com|x\.com`,
		"Yelp":      `yelp\.com`,
		"Pinterest": `pinterest\.com`,
	}
	var result []string
	for name, pattern := range platforms {
		re := regexp.MustCompile(`(?i)` + pattern)
		for _, l := range links {
			if re.MatchString(l.Href) {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Suspicious signal detection
// ─────────────────────────────────────────────────────────────────────────────

// DetectSuspiciousSignals scans for hidden links, spam keywords, and suspicious outbound domains.
func DetectSuspiciousSignals(html string, links []Link) SuspiciousResult {
	spamWords := []string{"casino", "gambling", "betting", "บาคาร่า", "viagra", "cialis", "poker", "porn", "payday loan", "slots", "escort"}

	hiddenRe := regexp.MustCompile(`(?i)<a[^>]+style=["'][^"']*(display\s*:\s*none|visibility\s*:\s*hidden|opacity\s*:\s*0)`)
	hiddenLinkCount := len(hiddenRe.FindAllString(html, -1))

	lower := strings.ToLower(html)
	var spamHits []string
	for _, w := range spamWords {
		if strings.Contains(lower, strings.ToLower(w)) {
			spamHits = append(spamHits, w)
		}
	}

	suspDomainRe := regexp.MustCompile(`(?i)casino|bet|gambling|poker|viagra|pharma|porn|escort|loan`)
	var suspDomains []string
	seen := make(map[string]bool)
	for _, l := range links {
		if suspDomainRe.MatchString(l.Href) {
			d := DomainFromURL(l.Href)
			if !seen[d] {
				seen[d] = true
				suspDomains = append(suspDomains, d)
			}
		}
	}

	risk := "low"
	if hiddenLinkCount > 0 || len(spamHits) > 0 || len(suspDomains) > 0 {
		if len(suspDomains) > 2 || len(spamHits) > 2 {
			risk = "high"
		} else {
			risk = "medium"
		}
	}

	var details string
	if risk == "low" {
		details = "No obvious spam keywords, hidden inline links, or suspicious outbound domains found in homepage HTML."
	} else {
		kws := "none"
		if len(spamHits) > 0 {
			kws = strings.Join(spamHits, ", ")
		}
		doms := "none"
		if len(suspDomains) > 0 {
			end := suspDomains
			if len(end) > 5 {
				end = end[:5]
			}
			doms = strings.Join(end, ", ")
		}
		details = "Potential risk found: " +
			string(rune('0'+hiddenLinkCount)) + " hidden link(s), keywords: " + kws +
			", domains: " + doms + "."
	}

	return SuspiciousResult{Risk: risk, Details: details}
}
