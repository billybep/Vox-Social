package seo

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Executive Summary builder
// ─────────────────────────────────────────────────────────────────────────────

// BuildExecutiveSummary produces the bullet-list summary shown in the dashboard.
// Ported from extension's buildExecutiveSummary().
func BuildExecutiveSummary(score int, scores map[string]int, passed, warnings, failed []Check, request AuditRequest, topIssues []Check, site SiteSummary) []string {
	// Find highest and lowest scoring categories
	type kv struct {
		Key string
		Val int
	}
	var sorted []kv
	for k, v := range scores {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Val < sorted[j].Val })

	lowest := sorted[0]
	highest := sorted[len(sorted)-1]

	summary := []string{
		fmt.Sprintf("The website received an overall score of %d/100, which gives it a grade of %s.", score, Grade(score)),
		fmt.Sprintf("%d checks passed, %d warnings were found, and %d high-risk failures were detected.", len(passed), len(warnings), len(failed)),
		fmt.Sprintf("Strongest area: %s (%d/100). Weakest area: %s (%d/100).", highest.Key, highest.Val, lowest.Key, lowest.Val),
	}

	if request.TargetKeyword != "" || request.TargetLocation != "" {
		parts := []string{request.TargetKeyword, request.TargetLocation}
		var filtered []string
		for _, p := range parts {
			if p != "" {
				filtered = append(filtered, p)
			}
		}
		summary = append(summary, fmt.Sprintf("The report reviewed local/keyword relevance using %s as the target context.", strings.Join(filtered, " in ")))
	}

	if len(topIssues) > 0 {
		summary = append(summary, fmt.Sprintf("Top priority: %s. This should be addressed first because it has %s SEO impact.", topIssues[0].Title, strings.ToLower(topIssues[0].Impact)))
	}

	sitemapCount := len(site.SitemapURLs)
	summary = append(summary, fmt.Sprintf("The audit scanned %d HTML page(s) found from %d XML sitemap file(s), with %d sitemap URL(s) unavailable or non-HTML.", site.PagesScanned, sitemapCount, site.FailedPages))
	summary = append(summary, fmt.Sprintf("Site-wide findings: %d missing meta descriptions, %d pages missing H1, %d thin pages, and %d images missing alt text.", site.MissingDescriptions, site.MissingH1, site.ThinPages, site.ImagesMissingAlt))
	summary = append(summary, "Advanced sections include PageSpeed/Core Web Vitals, mobile usability, social, technology, security headers, broken links, local listing API readiness, rankings, and backlink/domain authority data when APIs are connected.")
	summary = append(summary, "Backlink, live ranking, GBP performance, and full citation confirmation require connected APIs or manual review.")

	return summary
}

// ─────────────────────────────────────────────────────────────────────────────
// Action Plan builder
// ─────────────────────────────────────────────────────────────────────────────

type ActionPlanItem struct {
	Title       string
	Description string
}

// BuildActionPlan generates the 30-day priority action plan from top issues.
// Ported from extension's buildActionPlan().
func BuildActionPlan(issues []Check, request AuditRequest) []ActionPlanItem {
	defaults := []ActionPlanItem{
		{"Strengthen the homepage SEO foundation", "Rewrite the title, meta description, H1, and opening content around the primary service and target location."},
		{"Improve Local SEO signals", "Add consistent NAP details, LocalBusiness schema, Google Maps/GBP link, and clear service-area language."},
		{"Build service and location depth", "Create or expand service pages with FAQs, trust signals, internal links, reviews, and clear calls to action."},
		{"Optimize images and performance", "Compress images, convert to WebP/AVIF, lazy load below-the-fold assets, and reduce scripts/CSS bloat."},
		{"Clean up technical SEO", "Confirm robots.txt, XML sitemap, canonical tags, indexability, schema, and analytics tracking."},
		{"Improve authority and conversion", "Add stronger internal links, social proof, case studies, review acquisition, and citation consistency."},
	}

	var mapped []ActionPlanItem
	for _, issue := range issues {
		mapped = append(mapped, ActionPlanItem{
			Title:       issue.Title,
			Description: issue.Details + " Recommended fix: " + issue.Fix,
		})
	}

	// Merge: mapped first, then defaults for items not already present
	merged := append(mapped, defaults...)
	seen := make(map[string]bool)
	var unique []ActionPlanItem
	for _, item := range merged {
		if !seen[item.Title] {
			seen[item.Title] = true
			unique = append(unique, item)
		}
	}

	if len(unique) > 8 {
		return unique[:8]
	}
	return unique
}

// ─────────────────────────────────────────────────────────────────────────────
// Listings builder
// ─────────────────────────────────────────────────────────────────────────────

type ListingEntry struct {
	Platform string
	Status   string
	Link     string
	Note     string
}

// BuildListings produces the local citation readiness table.
// Ported from extension's buildListings().
func BuildListings(request AuditRequest, pageURL string, googlePlace GooglePlaceResult, yelp YelpResult) []ListingEntry {
	q := url.QueryEscape(strings.Join(filterEmpty([]string{
		request.BusinessName, request.TargetLocation, request.Phone,
	}), " "))
	if q == "" {
		q = url.QueryEscape(DomainFromURL(pageURL))
	}

	googleNote := buildGoogleNote(request, pageURL, googlePlace)
	googleStatus := "Verify Listing"
	googleLink := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%s", q)
	if googlePlace.Available {
		googleStatus = "API Found"
		googleLink = fmt.Sprintf("https://www.google.com/maps/search/%s", q)
	} else if request.GBPUrl != "" {
		googleStatus = "Provided"
		googleLink = request.GBPUrl
	}

	yelpNote := buildYelpNote(request, yelp)
	yelpStatus := "Verify Listing"
	yelpLink := fmt.Sprintf("https://www.yelp.com/search?find_desc=%s", q)
	if yelp.Available && len(yelp.Businesses) > 0 {
		yelpStatus = "API Found"
		yelpLink = yelp.Businesses[0].URL
	}

	return []ListingEntry{
		{"Google Business Profile", googleStatus, googleLink, googleNote},
		{"Yelp", yelpStatus, yelpLink, yelpNote},
		{"Bing Places / Maps", "Verify Listing", fmt.Sprintf("https://www.bing.com/maps?q=%s", q), "Manual/API verification recommended for Bing Places NAP and map listing consistency."},
		{"Apple Maps", "Verify Listing", fmt.Sprintf("https://maps.apple.com/?q=%s", q), "Manual verification recommended because Apple Business Connect data is not exposed via a public API."},
		{"Facebook Business Page", "Verify Listing", fmt.Sprintf("https://www.facebook.com/search/pages/?q=%s", q), "Verify brand name, phone, address, website, service category, and reviews/recommendations."},
		{"BBB", "Verify Listing", fmt.Sprintf("https://www.bbb.org/search?find_text=%s", q), "Verify whether a BBB profile exists and whether name, phone, and address match."},
		{"Yellow Pages", "Verify Listing", fmt.Sprintf("https://www.yellowpages.com/search?search_terms=%s", q), "Check old NAP, duplicate listings, and outdated phone numbers."},
		{"Angi / HomeAdvisor", "Verify Listing", fmt.Sprintf("https://www.google.com/search?q=%s+Angi+HomeAdvisor", q), "For home-service businesses, verify profile consistency and category relevance."},
		{"Chamber / Local Directories", "Verify Listing", fmt.Sprintf("https://www.google.com/search?q=%s+chamber+of+commerce+business+directory", q), "Look for local chamber, city directory, and niche citation opportunities."},
	}
}

func buildGoogleNote(request AuditRequest, pageURL string, gp GooglePlaceResult) string {
	if gp.Available {
		nameMatch := CompareText(request.BusinessName, gp.Name)
		phoneMatch := ComparePhone(request.Phone, gp.Phone)
		websiteMatch := CompareDomain(pageURL, gp.Website)
		return fmt.Sprintf("API match found. Name: %s • Phone: %s • Website: %s • Reviews: %d • Rating: %.1f.", nameMatch, phoneMatch, websiteMatch, gp.Reviews, gp.Rating)
	}
	if request.GBPUrl != "" {
		return "Google listing URL was provided. Connect Google Places API for rating, review, phone, address, category, and website matching."
	}
	return "Use this Maps search link or connect Google Places API for automated GBP confirmation."
}

func buildYelpNote(request AuditRequest, yelp YelpResult) string {
	if yelp.Available && len(yelp.Businesses) > 0 {
		b := yelp.Businesses[0]
		nameMatch := CompareText(request.BusinessName, b.Name)
		phoneMatch := ComparePhone(request.Phone, b.Phone)
		return fmt.Sprintf("API match found. Name: %s • Phone: %s • Reviews: %d • Rating: %.1f • Categories: %s.", nameMatch, phoneMatch, b.Reviews, b.Rating, b.Categories)
	}
	return "Use this Yelp search link or connect Yelp Fusion API for automated listing confirmation."
}

func filterEmpty(items []string) []string {
	var out []string
	for _, s := range items {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
