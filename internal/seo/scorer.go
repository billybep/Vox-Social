package seo

import (
	"math"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Page-level scoring
// ─────────────────────────────────────────────────────────────────────────────

type PageScoreParams struct {
	Title            string
	Description      string
	H1s              []string
	H2s              []string
	WordCount        int
	TotalImages      int
	ImagesMissingAlt int
	Canonical        string
	SchemaTypes      []string
	Noindex          bool
	InternalLinks    int
}

// ComputePageSEOScore computes a 0-100 score for a single page.
// Ported from extension's pageSeoScore().
func ComputePageSEOScore(p PageScoreParams) int {
	parts := []float64{}

	// Title: perfect 30-65 chars → 100, present but bad length → 65, missing → 0
	if p.Title != "" {
		if len(p.Title) >= 30 && len(p.Title) <= 65 {
			parts = append(parts, 100)
		} else {
			parts = append(parts, 65)
		}
	} else {
		parts = append(parts, 0)
	}

	// Description
	if p.Description != "" {
		if len(p.Description) >= 70 && len(p.Description) <= 165 {
			parts = append(parts, 100)
		} else {
			parts = append(parts, 65)
		}
	} else {
		parts = append(parts, 0)
	}

	// H1
	if len(p.H1s) == 1 {
		parts = append(parts, 100)
	} else if len(p.H1s) > 1 {
		parts = append(parts, 65)
	} else {
		parts = append(parts, 0)
	}

	// H2s
	if len(p.H2s) >= 2 {
		parts = append(parts, 100)
	} else {
		parts = append(parts, 55)
	}

	// Word count
	switch {
	case p.WordCount >= 700:
		parts = append(parts, 100)
	case p.WordCount >= 350:
		parts = append(parts, 70)
	default:
		parts = append(parts, 25)
	}

	// Images alt coverage
	if p.TotalImages > 0 {
		coverage := math.Max(0, float64(100-Percent(p.ImagesMissingAlt, p.TotalImages)))
		parts = append(parts, coverage)
	} else {
		parts = append(parts, 85)
	}

	// Canonical
	if p.Canonical != "" {
		parts = append(parts, 100)
	} else {
		parts = append(parts, 60)
	}

	// Schema
	if len(p.SchemaTypes) > 0 {
		parts = append(parts, 100)
	} else {
		parts = append(parts, 50)
	}

	// Indexability
	if p.Noindex {
		parts = append(parts, 0)
	} else {
		parts = append(parts, 100)
	}

	// Internal linking
	if p.InternalLinks >= 4 {
		parts = append(parts, 100)
	} else {
		parts = append(parts, 60)
	}

	sum := 0.0
	for _, v := range parts {
		sum += v
	}
	return int(math.Round(sum / float64(len(parts))))
}

// ─────────────────────────────────────────────────────────────────────────────
// Site-wide summary
// ─────────────────────────────────────────────────────────────────────────────

type SiteSummary struct {
	SitemapURLs           []string
	SitemapPageCount      int
	PagesScanned          int
	FailedPages           int
	AveragePageScore      int
	MissingTitles         int
	MissingDescriptions   int
	MissingH1             int
	MultipleH1            int
	ThinPages             int
	MissingCanonical      int
	NoindexPages          int
	PagesMissingSchema    int
	DuplicateTitles       int
	DuplicateDescriptions int
	TotalImages           int
	ImagesMissingAlt      int
	TotalInternalLinks    int
	TotalExternalLinks    int
	PagesWithLocalSignal  int
	PagesWithSocialProfiles int
	SuspiciousPages       int
	FailedPageURLs        []struct{ URL, Status, Error string }
	PageRows              []PageRow
	SocialProfiles        []string
	Technologies          []string
	SchemaTypes           []string
	TotalScripts          int
	TotalStylesheets      int
	TotalThirdPartyScripts int
}

type PageRow struct {
	URL       string
	Score     int
	WordCount int
	Issues    string
}

// BuildSiteSummary aggregates page-level analyses into site-wide statistics.
// Ported from extension's buildSiteSummary().
func BuildSiteSummary(pageAnalyses []PageAnalysis, sitemapPageURLs []string, sitemapURLs []string) SiteSummary {
	var htmlPages, failedPages []PageAnalysis
	for _, p := range pageAnalyses {
		if p.OK {
			htmlPages = append(htmlPages, p)
		} else {
			failedPages = append(failedPages, p)
		}
	}

	scoreSum := 0
	for _, p := range htmlPages {
		scoreSum += p.Score
	}
	avgScore := 0
	if len(htmlPages) > 0 {
		avgScore = scoreSum / len(htmlPages)
	}

	// Collect tech/social/schema sets
	techSet := make(map[string]bool)
	socialSet := make(map[string]bool)
	schemaSet := make(map[string]bool)

	totalImages := 0
	imagesMissingAlt := 0
	totalInternal := 0
	totalExternal := 0
	totalScripts := 0
	totalStylesheets := 0
	totalThirdParty := 0
	localSignalPages := 0
	socialProfilePages := 0
	suspiciousPages := 0

	titleList := make([]string, 0, len(htmlPages))
	descList := make([]string, 0, len(htmlPages))

	for _, p := range htmlPages {
		titleList = append(titleList, p.Title)
		descList = append(descList, p.Description)
		for _, t := range p.Tech {
			techSet[t] = true
		}
		for _, s := range p.SocialProfiles {
			socialSet[s] = true
		}
		for _, s := range p.SchemaTypes {
			schemaSet[s] = true
		}
		totalImages += p.Images
		imagesMissingAlt += p.ImagesMissingAlt
		totalInternal += p.InternalLinks
		totalExternal += p.ExternalLinks
		totalScripts += p.Scripts
		totalStylesheets += p.Stylesheets
		totalThirdParty += p.ThirdPartyScripts
		if len(p.PhoneMatches) > 0 || p.AddressHints || p.MapEmbed {
			localSignalPages++
		}
		if len(p.SocialProfiles) > 0 {
			socialProfilePages++
		}
		if p.Suspicious.Risk != "low" {
			suspiciousPages++
		}
	}

	// Page rows sorted by score ascending (worst first)
	rows := make([]PageRow, 0, len(htmlPages))
	for _, p := range htmlPages {
		issueStr := strings.Join(p.Issues, ", ")
		if issueStr == "" {
			issueStr = "No major issue"
		}
		rows = append(rows, PageRow{
			URL:       p.URL,
			Score:     p.Score,
			WordCount: p.WordCount,
			Issues:    issueStr,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Score < rows[j].Score })

	// Cap at 100 rows for the report
	if len(rows) > 100 {
		rows = rows[:100]
	}

	failedPageURLs := make([]struct{ URL, Status, Error string }, 0, Min(50, len(failedPages)))
	for _, p := range failedPages {
		if len(failedPageURLs) >= 50 {
			break
		}
		failedPageURLs = append(failedPageURLs, struct{ URL, Status, Error string }{
			URL:    p.URL,
			Status: strings.Repeat("0", 0), // fill below
			Error:  p.Error,
		})
	}

	toSlice := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}

	s := SiteSummary{
		SitemapURLs:            sitemapURLs,
		SitemapPageCount:       len(sitemapPageURLs),
		PagesScanned:           len(htmlPages),
		FailedPages:            len(failedPages),
		AveragePageScore:       avgScore,
		MissingTitles:          countWhere(htmlPages, func(p PageAnalysis) bool { return p.Title == "" }),
		MissingDescriptions:    countWhere(htmlPages, func(p PageAnalysis) bool { return p.Description == "" }),
		MissingH1:              countWhere(htmlPages, func(p PageAnalysis) bool { return len(p.H1s) == 0 }),
		MultipleH1:             countWhere(htmlPages, func(p PageAnalysis) bool { return len(p.H1s) > 1 }),
		ThinPages:              countWhere(htmlPages, func(p PageAnalysis) bool { return p.WordCount < 350 }),
		MissingCanonical:       countWhere(htmlPages, func(p PageAnalysis) bool { return p.Canonical == "" }),
		NoindexPages:           countWhere(htmlPages, func(p PageAnalysis) bool { return strings.Contains(strings.ToLower(p.RobotsMeta), "noindex") }),
		PagesMissingSchema:     countWhere(htmlPages, func(p PageAnalysis) bool { return len(p.SchemaTypes) == 0 }),
		DuplicateTitles:        countDuplicates(titleList),
		DuplicateDescriptions:  countDuplicates(descList),
		TotalImages:            totalImages,
		ImagesMissingAlt:       imagesMissingAlt,
		TotalInternalLinks:     totalInternal,
		TotalExternalLinks:     totalExternal,
		PagesWithLocalSignal:   localSignalPages,
		PagesWithSocialProfiles: socialProfilePages,
		SuspiciousPages:        suspiciousPages,
		FailedPageURLs:         failedPageURLs,
		PageRows:               rows,
		SocialProfiles:         toSlice(socialSet),
		Technologies:           toSlice(techSet),
		SchemaTypes:            toSlice(schemaSet),
		TotalScripts:           totalScripts,
		TotalStylesheets:       totalStylesheets,
		TotalThirdPartyScripts: totalThirdParty,
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// Category scoring
// ─────────────────────────────────────────────────────────────────────────────

type Check struct {
	Category string
	Title    string
	Status   string // pass | warn | fail | info
	Details  string
	Fix      string
	Impact   string // High | Medium | Low
}

// ComputeCategoryScores averages the status weights per category.
// Ported from extension's category scoring loop.
func ComputeCategoryScores(checks []Check) map[string]int {
	categories := []string{"SEO", "Usability", "Performance", "Social", "Local SEO", "Technology", "Links", "Rankings", "Authority"}
	scores := make(map[string]int, len(categories)+1)

	for _, cat := range categories {
		var catChecks []Check
		for _, c := range checks {
			if c.Category == cat && c.Status != "info" {
				catChecks = append(catChecks, c)
			}
		}
		if len(catChecks) == 0 {
			scores[cat] = 75
			continue
		}
		sum := 0
		for _, c := range catChecks {
			sum += StatusWeight(c.Status)
		}
		scores[cat] = int(math.Round(float64(sum) / float64(len(catChecks))))
	}

	// Security sub-score from 3 specific checks
	findStatus := func(title string) string {
		for _, c := range checks {
			if c.Title == title {
				return c.Status
			}
		}
		return "warn"
	}
	scores["Security"] = int(math.Round(float64(
		StatusWeight(findStatus("HTTPS Security"))+
			StatusWeight(findStatus("Spam / Suspicious Link Risk"))+
			StatusWeight(findStatus("Security Headers"))) / 3.0))

	return scores
}

// ComputeOverallScore applies weighted category scores.
// Ported from extension's overallScore formula.
func ComputeOverallScore(scores map[string]int) int {
	weights := map[string]float64{
		"SEO":         0.23,
		"Usability":   0.10,
		"Performance": 0.15,
		"Social":      0.07,
		"Local SEO":   0.17,
		"Technology":  0.08,
		"Links":       0.08,
		"Rankings":    0.04,
		"Authority":   0.04,
		"Security":    0.04,
	}
	total := 0.0
	for cat, w := range weights {
		total += float64(scores[cat]) * w
	}
	return int(math.Round(total))
}

// ─────────────────────────────────────────────────────────────────────────────
// Site-wide aggregate signals
// ─────────────────────────────────────────────────────────────────────────────

type AggregateSignals struct {
	Tech              []string
	Social            []string
	Schema            []string
	ExternalDomains   []string
	Forms             int
	Scripts           int
	Stylesheets       int
	ThirdPartyScripts int
	MapPages          int
	PhonePages        int
}

// AggregateSiteSignals collects cross-page technology/social signals.
func AggregateSiteSignals(pageAnalyses []PageAnalysis) AggregateSignals {
	techSet := make(map[string]bool)
	socialSet := make(map[string]bool)
	schemaSet := make(map[string]bool)
	extDomainSet := make(map[string]bool)
	forms, scripts, stylesheets, thirdParty, mapPages, phonePages := 0, 0, 0, 0, 0, 0

	for _, p := range pageAnalyses {
		if !p.OK {
			continue
		}
		for _, t := range p.Tech {
			techSet[t] = true
		}
		for _, s := range p.SocialProfiles {
			socialSet[s] = true
		}
		for _, s := range p.SchemaTypes {
			schemaSet[s] = true
		}
		for _, d := range p.UniqueExternalDomains {
			extDomainSet[d] = true
		}
		forms += p.Forms
		scripts += p.Scripts
		stylesheets += p.Stylesheets
		thirdParty += p.ThirdPartyScripts
		if p.MapEmbed {
			mapPages++
		}
		if len(p.PhoneMatches) > 0 {
			phonePages++
		}
	}

	toSlice := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	return AggregateSignals{
		Tech:              toSlice(techSet),
		Social:            toSlice(socialSet),
		Schema:            toSlice(schemaSet),
		ExternalDomains:   toSlice(extDomainSet),
		Forms:             forms,
		Scripts:           scripts,
		Stylesheets:       stylesheets,
		ThirdPartyScripts: thirdParty,
		MapPages:          mapPages,
		PhonePages:        phonePages,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func countWhere(pages []PageAnalysis, fn func(PageAnalysis) bool) int {
	count := 0
	for _, p := range pages {
		if fn(p) {
			count++
		}
	}
	return count
}

func countDuplicates(items []string) int {
	freq := make(map[string]int)
	for _, s := range items {
		if s = strings.TrimSpace(strings.ToLower(s)); s != "" {
			freq[s]++
		}
	}
	total := 0
	for _, v := range freq {
		if v > 1 {
			total += v
		}
	}
	return total
}
