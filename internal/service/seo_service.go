package service

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/voxlumedia/vox-social-backend/internal/model"
	"github.com/voxlumedia/vox-social-backend/internal/seo"
)

// SEOConfig holds all optional API keys for the SEO audit pipeline.
type SEOConfig struct {
	PageSpeedAPIKey    string
	AIApiKey           string
	AIApiEndpoint      string
	AIModel            string
	GooglePlacesAPIKey string
	YelpAPIKey         string
	DataForSEOLogin    string
	DataForSEOPassword string
	MaxSitemapPages    int
	BrokenLinkLimit    int
}

// SEOService orchestrates the full SEO audit pipeline.
type SEOService struct {
	cfg  SEOConfig
	stor *StorageService
}

// NewSEOService creates a new SEOService.
func NewSEOService(cfg SEOConfig, stor *StorageService) *SEOService {
	return &SEOService{cfg: cfg, stor: stor}
}

// RunAudit is the main entry point. It mirrors the extension's runAudit().
func (s *SEOService) RunAudit(ctx context.Context, req model.SEOAuditRequest) (*model.SEOAuditResponse, error) {
	log.Printf("[SEO] Starting audit for domain: %s", req.Domain)

	// ── 1. Normalize URL ───────────────────────────────────────────────────
	websiteURL, err := seo.NormalizeURL(req.Domain)
	if err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}
	base, err := url.Parse(websiteURL)
	if err != nil {
		return nil, fmt.Errorf("URL parse error: %w", err)
	}
	origin := base.Scheme + "://" + base.Host
	domain := seo.DomainFromURL(websiteURL)

	auditReq := seo.AuditRequest{
		WebsiteURL:     websiteURL,
		BusinessName:   req.BusinessName,
		TargetKeyword:  req.TargetKeyword,
		TargetLocation: req.TargetLocation,
		Phone:          req.Phone,
		GBPUrl:         req.GBPUrl,
	}

	// ── 2. Fetch homepage ──────────────────────────────────────────────────
	log.Printf("[SEO] Fetching homepage...")
	home := seo.FetchText(ctx, websiteURL, 20*time.Second, "GET")
	if home.Error != "" {
		return nil, fmt.Errorf("network error fetching homepage: %s", home.Error)
	}
	if home.Text == "" {
		return nil, fmt.Errorf("could not fetch the website homepage (empty response, status %d)", home.Status)
	}

	// ── 3. Fetch robots.txt + common sitemaps in parallel ─────────────────
	log.Printf("[SEO] Fetching robots.txt and sitemaps...")
	commonSitemapCandidates := []string{
		origin + "/sitemap.xml",
		origin + "/sitemap_index.xml",
		origin + "/wp-sitemap.xml",
		origin + "/page-sitemap.xml",
		origin + "/post-sitemap.xml",
	}

	robotsCh := make(chan seo.FetchResult, 1)
	sitemapChs := make([]chan seo.FetchResult, len(commonSitemapCandidates))
	for i := range commonSitemapCandidates {
		sitemapChs[i] = make(chan seo.FetchResult, 1)
	}

	go func() { robotsCh <- seo.FetchText(ctx, origin+"/robots.txt", 9*time.Second, "GET") }()
	for i, sc := range commonSitemapCandidates {
		i, sc := i, sc
		go func() { sitemapChs[i] <- seo.FetchText(ctx, sc, 12*time.Second, "GET") }()
	}

	robots := <-robotsCh
	commonSitemaps := make([]seo.FetchResult, len(commonSitemapCandidates))
	for i := range commonSitemapCandidates {
		commonSitemaps[i] = <-sitemapChs[i]
	}

	// ── 4. Discover sitemap pages ─────────────────────────────────────────
	log.Printf("[SEO] Discovering sitemap pages...")
	discovery := seo.DiscoverSitemapPages(ctx, origin, robots.Text, commonSitemaps)
	sitemapPageURLs := discovery.PageURLs
	if len(sitemapPageURLs) == 0 {
		sitemapPageURLs = []string{home.URL}
	}

	// Ensure homepage is included
	homeKey := seo.ComparableURL(home.URL)
	hasHome := false
	for _, u := range sitemapPageURLs {
		if seo.ComparableURL(u) == homeKey {
			hasHome = true
			break
		}
	}
	if !hasHome {
		sitemapPageURLs = append([]string{home.URL}, sitemapPageURLs...)
	}

	// Cap at MaxSitemapPages
	limit := s.cfg.MaxSitemapPages
	if limit > 0 && len(sitemapPageURLs) > limit {
		sitemapPageURLs = sitemapPageURLs[:limit]
	}

	mainSitemap := ""
	if len(discovery.SitemapURLs) > 0 {
		mainSitemap = discovery.SitemapURLs[0]
	} else {
		for _, cs := range commonSitemaps {
			if cs.OK {
				mainSitemap = cs.URL
				break
			}
		}
	}

	// ── 5. Scan all sitemap pages concurrently ────────────────────────────
	log.Printf("[SEO] Scanning %d sitemap pages...", len(sitemapPageURLs))
	pageAnalyses := seo.ScanSitemapPages(ctx, sitemapPageURLs, home.URL, home, auditReq, origin)

	// ── 6. Build site-wide summary ────────────────────────────────────────
	site := seo.BuildSiteSummary(pageAnalyses, sitemapPageURLs, discovery.SitemapURLs)
	aggregate := seo.AggregateSiteSignals(pageAnalyses)

	// ── 7. Analyze homepage specifically ─────────────────────────────────
	var homePage *seo.PageAnalysis
	for i := range pageAnalyses {
		if seo.ComparableURL(pageAnalyses[i].URL) == homeKey && pageAnalyses[i].OK {
			homePage = &pageAnalyses[i]
			break
		}
	}
	if homePage == nil {
		for i := range pageAnalyses {
			if pageAnalyses[i].OK {
				homePage = &pageAnalyses[i]
				break
			}
		}
	}
	if homePage == nil {
		// Analyze inline from fetched text
		analysis := seo.AnalyzeHTMLPage(websiteURL, home, auditReq, origin)
		homePage = &analysis
	}

	securityHeaders := seo.AnalyzeSecurityHeaders(home.Headers)

	// ── 8. Run advanced modules in parallel ───────────────────────────────
	log.Printf("[SEO] Running advanced modules...")
	var (
		brokenLinks seo.BrokenLinksResult
		psiMobile   seo.PSIResult
		psiDesktop  seo.PSIResult
		googlePlace seo.GooglePlaceResult
		yelpResult  seo.YelpResult
		rankings    seo.RankingsResult
		backlinks   seo.BacklinksResult
	)
	var wg sync.WaitGroup
	wg.Add(6)
	go func() { defer wg.Done(); brokenLinks = seo.CheckBrokenLinks(ctx, pageAnalyses, s.cfg.BrokenLinkLimit) }()
	go func() {
		defer wg.Done()
		psiMobile = seo.FetchPageSpeed(ctx, websiteURL, "mobile", s.cfg.PageSpeedAPIKey)
	}()
	go func() {
		defer wg.Done()
		psiDesktop = seo.FetchPageSpeed(ctx, websiteURL, "desktop", s.cfg.PageSpeedAPIKey)
	}()
	go func() { defer wg.Done(); googlePlace = seo.FetchGooglePlace(ctx, auditReq, s.cfg.GooglePlacesAPIKey) }()
	go func() { defer wg.Done(); yelpResult = seo.FetchYelpBusiness(ctx, auditReq, s.cfg.YelpAPIKey) }()
	go func() {
		defer wg.Done()
		rankings = seo.FetchDataForSEORankings(ctx, auditReq, s.cfg.DataForSEOLogin, s.cfg.DataForSEOPassword)
		backlinks = seo.FetchDataForSEOBacklinks(ctx, auditReq, s.cfg.DataForSEOLogin, s.cfg.DataForSEOPassword)
	}()
	wg.Wait()

	// ── 9. Build all audit checks ─────────────────────────────────────────
	log.Printf("[SEO] Building audit checks...")
	checks := buildChecks(homePage, &robots, mainSitemap, &site, &aggregate, &securityHeaders, &psiMobile, &psiDesktop, &brokenLinks, &googlePlace, &yelpResult, &rankings, &backlinks, auditReq, base, home.Text)

	// ── 10. Score ─────────────────────────────────────────────────────────
	categoryScores := seo.ComputeCategoryScores(checks)
	overallScore := seo.ComputeOverallScore(categoryScores)

	var passed, warnings, failed []seo.Check
	for _, c := range checks {
		switch c.Status {
		case "pass":
			passed = append(passed, c)
		case "warn":
			warnings = append(warnings, c)
		case "fail":
			failed = append(failed, c)
		}
	}

	// Top issues = failed + warn, sorted by impact
	topIssues := append(append([]seo.Check{}, failed...), warnings...)
	sort.Slice(topIssues, func(i, j int) bool {
		return seo.ImpactRank(topIssues[i].Impact) > seo.ImpactRank(topIssues[j].Impact)
	})
	if len(topIssues) > 6 {
		topIssues = topIssues[:6]
	}

	// ── 11. Build report content ──────────────────────────────────────────
	execSummary := seo.BuildExecutiveSummary(overallScore, categoryScores, passed, warnings, failed, auditReq, topIssues, site)
	actionPlan := seo.BuildActionPlan(topIssues, auditReq)
	listings := seo.BuildListings(auditReq, websiteURL, googlePlace, yelpResult)

	// ── 12. AI recommendations ────────────────────────────────────────────
	log.Printf("[SEO] Fetching AI recommendations...")
	var psiMobileScore, psiDesktopScore *int
	if psiMobile.Available {
		v := psiMobile.Performance
		psiMobileScore = &v
	}
	if psiDesktop.Available {
		v := psiDesktop.Performance
		psiDesktopScore = &v
	}
	aiRecs := seo.FetchAIRecommendations(ctx, seo.AuditSnapshot{
		URL:          websiteURL,
		BusinessName: req.BusinessName,
		Keyword:      req.TargetKeyword,
		Location:     req.TargetLocation,
		OverallScore: overallScore,
		Grade:        seo.Grade(overallScore),
		Scores:       categoryScores,
		TopIssues:    topIssues,
		PSIMobile:    psiMobileScore,
		PSIDesktop:   psiDesktopScore,
		BrokenLinks:  brokenLinks.Broken,
	}, s.cfg.AIApiKey, s.cfg.AIModel, s.cfg.AIApiEndpoint)

	if aiRecs.Available {
		if len(aiRecs.ExecutiveSummary) > 0 {
			execSummary = aiRecs.ExecutiveSummary
		}
		if len(aiRecs.ActionPlan) > 0 {
			actionPlan = aiRecs.ActionPlan
		}
	}

	// ── 13. Assemble response ─────────────────────────────────────────────
	resp := assembleResponse(
		uuid.New(), domain, websiteURL, overallScore, categoryScores,
		passed, warnings, failed,
		execSummary, actionPlan, listings,
		homePage, &site, &brokenLinks, &psiMobile, &psiDesktop,
		&aggregate, &securityHeaders,
		checks, aiRecs,
	)

	// ── 14. Save async ────────────────────────────────────────────────────
	go s.stor.SaveSEOAudit(resp)

	log.Printf("[SEO] Audit complete for %s → score=%d grade=%s", domain, overallScore, seo.Grade(overallScore))
	return resp, nil
}

// GetAuditByID loads a full audit report from the database.
func (s *SEOService) GetAuditByID(ctx context.Context, id string) (*model.SEOAuditResponse, error) {
	return s.stor.GetSEOAuditByID(ctx, id)
}

// GetAuditsByDomain returns audit history summaries for a domain.
func (s *SEOService) GetAuditsByDomain(ctx context.Context, domain string) ([]model.SEOAuditSummary, error) {
	return s.stor.GetSEOAuditsByDomain(ctx, domain)
}

// ─────────────────────────────────────────────────────────────────────────────
// Check builder — ports all add() calls from extension's runAudit()
// ─────────────────────────────────────────────────────────────────────────────

func buildChecks(
	hp *seo.PageAnalysis,
	robots *seo.FetchResult,
	mainSitemap string,
	site *seo.SiteSummary,
	agg *seo.AggregateSignals,
	sec *seo.SecurityHeaders,
	psiM, psiD *seo.PSIResult,
	bl *seo.BrokenLinksResult,
	gp *seo.GooglePlaceResult,
	yelp *seo.YelpResult,
	rankings *seo.RankingsResult,
	backlinks *seo.BacklinksResult,
	req seo.AuditRequest,
	base *url.URL,
	homeHTML string,
) []seo.Check {
	var checks []seo.Check
	add := func(category, title, status, details, fix, impact string) {
		checks = append(checks, seo.Check{Category: category, Title: title, Status: status, Details: details, Fix: fix, Impact: impact})
	}

	title := hp.Title
	description := hp.Description
	canonical := hp.Canonical
	robotsMeta := hp.RobotsMeta
	viewport := hp.Viewport
	lang := hp.Lang
	h1s := hp.H1s
	h2s := hp.H2s
	h3s := hp.H3s
	wc := hp.WordCount
	images := hp.Images
	lazyImages := hp.LazyImages
	scripts := hp.Scripts
	stylesheets := hp.Stylesheets
	thirdPartyScripts := hp.ThirdPartyScripts
	internalLinks := hp.InternalLinks
	externalLinks := hp.ExternalLinks
	schemaTypes := hp.SchemaTypes
	ogTitle := hp.OGTitle
	ogDesc := hp.OGDesc
	ogImage := hp.OGImage
	twitterCard := hp.TwitterCard
	favicon := hp.Favicon
	tech := hp.Tech
	socialProfiles := hp.SocialProfiles
	phoneMatches := hp.PhoneMatches
	emailMatches := hp.EmailMatches
	addressHints := hp.AddressHints
	mapEmbed := hp.MapEmbed
	businessNamePresent := hp.BusinessNamePresent
	keywordPresent := hp.KeywordPresent
	locationPresent := hp.LocationPresent
	phonePresent := hp.PhonePresent
	suspicious := hp.Suspicious
	htmlSizeKB := hp.HTMLSizeKB

	robotsOrigin := base.Scheme + "://" + base.Host
	robotsAllows := robots.OK && !regexp.MustCompile(`(?i)disallow:\s*/`).MatchString(robots.Text)

	// ── SEO checks ────────────────────────────────────────────────────────
	titleStatus := "fail"
	titleDetails := "No title tag was found."
	if title != "" {
		if len(title) >= 30 && len(title) <= 65 {
			titleStatus = "pass"
		} else {
			titleStatus = "warn"
		}
		titleDetails = fmt.Sprintf("%d characters: %s", len(title), title)
	}
	add("SEO", "Title Tag", titleStatus, titleDetails, "Keep primary keyword and brand within 30-65 characters.", "High")

	descStatus := "fail"
	descDetails := "No meta description was found."
	if description != "" {
		if len(description) >= 70 && len(description) <= 165 {
			descStatus = "pass"
		} else {
			descStatus = "warn"
		}
		descDetails = fmt.Sprintf("%d characters: %s", len(description), description)
	}
	add("SEO", "Meta Description", descStatus, descDetails, "Rewrite the description to be compelling and within 70-165 characters.", "High")

	canonicalStatus := "warn"
	canonicalDetails := "No canonical tag detected."
	if canonical != "" {
		canonicalStatus = "pass"
		canonicalDetails = "Canonical URL found: " + canonical
	}
	add("SEO", "Canonical Tag", canonicalStatus, canonicalDetails, "Add a self-referencing canonical tag to help prevent duplicate URL issues.", "Medium")

	robotsMetaStatus := "pass"
	robotsMetaDetails := robotsMeta
	if robotsMetaDetails == "" {
		robotsMetaDetails = "No blocking meta robots directive found."
	}
	if regexp.MustCompile(`(?i)noindex|nofollow`).MatchString(robotsMeta) {
		robotsMetaStatus = "fail"
	}
	add("SEO", "Meta Robots", robotsMetaStatus, robotsMetaDetails, "Remove accidental noindex/nofollow directives from important pages.", "High")

	robotsTxtStatus := "warn"
	robotsTxtDetails := "robots.txt was not accessible."
	if robots.OK {
		robotsTxtStatus = "pass"
		robotsTxtDetails = fmt.Sprintf("robots.txt found at %s/robots.txt", robotsOrigin)
	}
	add("SEO", "Robots.txt", robotsTxtStatus, robotsTxtDetails, "Add a clean robots.txt file and include the XML sitemap location.", "Medium")

	sitemapStatus := "fail"
	sitemapDetails := "No XML sitemap location was detected."
	if mainSitemap != "" {
		sitemapStatus = "pass"
		sitemapDetails = "Sitemap detected: " + mainSitemap
	}
	add("SEO", "XML Sitemap", sitemapStatus, sitemapDetails, "Create and submit an XML sitemap in Google Search Console.", "High")

	indexabilityStatus := "pass"
	if !robotsAllows {
		indexabilityStatus = "warn"
	}
	indexDetails := "robots.txt does not appear to block the full site for all crawlers."
	if !robots.OK {
		indexDetails = "Could not confirm robots.txt crawl rules."
	}
	add("SEO", "Indexability Signals", indexabilityStatus, indexDetails, "Make sure important pages are crawlable and indexable.", "High")

	coverageStatus := "fail"
	if site.PagesScanned > 0 {
		if site.PagesScanned == site.SitemapPageCount {
			coverageStatus = "pass"
		} else {
			coverageStatus = "warn"
		}
	}
	add("SEO", "Sitemap Page Coverage", coverageStatus,
		fmt.Sprintf("%d of %d sitemap page URL(s) were scanned as HTML pages.", site.PagesScanned, site.SitemapPageCount),
		"Review failed/non-HTML sitemap URLs and make sure all important URLs are accessible.", "High")

	sitewideTDStatus := "pass"
	if site.MissingTitles > 0 || site.MissingDescriptions > 0 || site.DuplicateTitles > 0 || site.DuplicateDescriptions > 0 {
		sitewideTDStatus = "warn"
	}
	add("SEO", "Sitewide Titles & Descriptions", sitewideTDStatus,
		fmt.Sprintf("%d missing titles, %d missing descriptions, %d duplicate title entries, %d duplicate description entries across sitemap pages.", site.MissingTitles, site.MissingDescriptions, site.DuplicateTitles, site.DuplicateDescriptions),
		"Create unique SEO titles and meta descriptions for every indexable sitemap page.", "High")

	headingStatus := "pass"
	if site.MissingH1 > 0 || site.MultipleH1 > 0 {
		headingStatus = "warn"
	}
	add("SEO", "Sitewide Heading Structure", headingStatus,
		fmt.Sprintf("%d page(s) missing H1 and %d page(s) with multiple H1 tags.", site.MissingH1, site.MultipleH1),
		"Use one clear H1 on each sitemap page and organize sections with H2/H3 headings.", "High")

	thinStatus := "pass"
	if site.ThinPages > 0 {
		thinStatus = "warn"
	}
	add("SEO", "Sitewide Thin Content", thinStatus,
		fmt.Sprintf("%d sitemap page(s) have fewer than 350 visible words.", site.ThinPages),
		"Expand weak pages with service detail, FAQs, proof, internal links, and local relevance.", "High")

	canonicalCovStatus := "pass"
	if site.MissingCanonical > 0 {
		canonicalCovStatus = "warn"
	}
	add("SEO", "Sitewide Canonical Coverage", canonicalCovStatus,
		fmt.Sprintf("%d sitemap page(s) are missing canonical tags.", site.MissingCanonical),
		"Add self-referencing canonical tags to every important page.", "Medium")

	altCovStatus := "pass"
	if site.ImagesMissingAlt > 0 {
		altCovStatus = "warn"
	}
	add("SEO", "Sitewide Image Alt Coverage", altCovStatus,
		fmt.Sprintf("%d of %d image(s) across sitemap pages are missing alt text.", site.ImagesMissingAlt, site.TotalImages),
		"Add descriptive alt text to important images across all scanned sitemap pages.", "Medium")

	schemaCovStatus := "pass"
	if site.PagesMissingSchema > 0 {
		schemaCovStatus = "warn"
	}
	add("SEO", "Sitewide Schema Coverage", schemaCovStatus,
		fmt.Sprintf("%d of %d scanned sitemap page(s) have no detected schema markup.", site.PagesMissingSchema, site.PagesScanned),
		"Add relevant Organization, LocalBusiness, Service, Article, FAQ, and Breadcrumb schema where appropriate.", "Medium")

	// On-page
	h1Status := "fail"
	h1Details := "No H1 tag found."
	if len(h1s) == 1 {
		h1Status = "pass"
		h1Details = "1 H1 tag found: " + h1s[0]
	} else if len(h1s) > 1 {
		h1Status = "warn"
		h1Details = fmt.Sprintf("%d H1 tags found: %s", len(h1s), strings.Join(h1s[:seo.Min(3, len(h1s))], " | "))
	}
	add("SEO", "H1 Tag", h1Status, h1Details, "Use one clear H1 that describes the page topic.", "High")

	h2Status := "pass"
	if len(h2s) < 2 {
		h2Status = "warn"
	}
	add("SEO", "Heading Structure", h2Status,
		fmt.Sprintf("%d H2 tags and %d H3 tags found.", len(h2s), len(h3s)),
		"Use H2/H3 sections to structure services, benefits, FAQs, and location topics.", "Medium")

	contentStatus := "fail"
	switch {
	case wc >= 700:
		contentStatus = "pass"
	case wc >= 350:
		contentStatus = "warn"
	}
	add("SEO", "Content Depth", contentStatus,
		fmt.Sprintf("%d visible words detected on the homepage.", wc),
		"Build deeper service content, FAQs, proof, process, and location relevance.", "High")

	if req.TargetKeyword != "" {
		kwStatus := "warn"
		kwDetails := fmt.Sprintf("Target keyword not found exactly: %s", req.TargetKeyword)
		if keywordPresent {
			kwStatus = "pass"
			kwDetails = fmt.Sprintf("Target keyword appears on the page: %s", req.TargetKeyword)
		}
		add("SEO", "Target Keyword Usage", kwStatus, kwDetails, "Use the target keyword naturally in the title, H1, first paragraph, and one H2.", "High")
	}

	ilStatus := "pass"
	if internalLinks < 8 {
		ilStatus = "warn"
	}
	add("SEO", "Internal Linking", ilStatus,
		fmt.Sprintf("%d internal links found on the homepage.", internalLinks),
		"Add descriptive internal links to main service, location, blog, and contact pages.", "Medium")

	elStatus := "pass"
	if externalLinks > 35 {
		elStatus = "warn"
	}
	add("SEO", "External Linking", elStatus,
		fmt.Sprintf("%d external links found.", externalLinks),
		"Keep outbound links relevant and avoid linking to unrelated domains.", "Low")

	// ── Usability ─────────────────────────────────────────────────────────
	vpStatus := "fail"
	vpDetails := "No viewport meta tag detected."
	if viewport != "" {
		vpStatus = "pass"
		vpDetails = viewport
	}
	add("Usability", "Mobile Viewport", vpStatus, vpDetails, "Add a responsive viewport tag for mobile usability.", "High")

	langStatus := "warn"
	langDetails := "No HTML lang attribute found."
	if lang != "" {
		langStatus = "pass"
		langDetails = "HTML lang attribute: " + lang
	}
	add("Usability", "Language Attribute", langStatus, langDetails, "Add a language attribute like <html lang=\"en\">.", "Low")

	contactStatus := "warn"
	if len(phoneMatches) > 0 || len(emailMatches) > 0 || hp.Forms > 0 {
		contactStatus = "pass"
	}
	add("Usability", "Contact Signals", contactStatus,
		fmt.Sprintf("%d phone(s), %d email(s), %d form(s) found.", len(phoneMatches), len(emailMatches), hp.Forms),
		"Make phone, email, and forms easy to access above the fold and in the footer.", "Medium")

	faviconStatus := "warn"
	faviconDetails := "No favicon/touch icon detected."
	if favicon != "" {
		faviconStatus = "pass"
		faviconDetails = "Favicon or touch icon detected."
	}
	add("Usability", "Favicon / App Icon", faviconStatus, faviconDetails, "Add favicon and app icons to improve brand trust.", "Low")

	ctaStatus := "warn"
	if seo.ContainsAny(hp.PageText, []string{"call", "quote", "contact", "book", "schedule", "estimate", "get started", "request"}) {
		ctaStatus = "pass"
	}
	add("Usability", "CTA / Conversion Terms", ctaStatus, "CTA language check completed from homepage text.", "Use clear CTA sections such as Get a Free Quote, Call Now, or Schedule Service.", "Medium")

	// ── Performance ───────────────────────────────────────────────────────
	htmlSizeStatus := "pass"
	if htmlSizeKB > 600 {
		htmlSizeStatus = "fail"
	} else if htmlSizeKB > 250 {
		htmlSizeStatus = "warn"
	}
	add("Performance", "HTML Page Size", htmlSizeStatus,
		fmt.Sprintf("Homepage HTML response is about %d KB.", htmlSizeKB),
		"Reduce bloated markup, unused builder output, and unnecessary embedded scripts.", "Medium")

	jsStatus := "pass"
	if scripts > 45 {
		jsStatus = "fail"
	} else if scripts > 25 {
		jsStatus = "warn"
	}
	add("Performance", "JavaScript Load", jsStatus,
		fmt.Sprintf("%d script tags detected; %d are third-party scripts.", scripts, thirdPartyScripts),
		"Remove unused scripts, defer non-critical JS, and reduce third-party tags.", "High")

	cssStatus := "pass"
	if stylesheets > 30 {
		cssStatus = "fail"
	} else if stylesheets > 15 {
		cssStatus = "warn"
	}
	add("Performance", "CSS Load", cssStatus,
		fmt.Sprintf("%d stylesheet files detected.", stylesheets),
		"Combine/reduce CSS where possible and load only page-specific assets.", "Medium")

	imgOptStatus := "pass"
	if images > 0 {
		if seo.Percent(lazyImages, images) < 50 {
			imgOptStatus = "warn"
		}
	}
	add("Performance", "Image Optimization Signals", imgOptStatus,
		fmt.Sprintf("%d images found; %d use native lazy loading.", images, lazyImages),
		"Compress images, use WebP/AVIF, and lazy load below-the-fold images.", "High")

	// ── Social ────────────────────────────────────────────────────────────
	ogStatus := "warn"
	if ogTitle != "" && ogDesc != "" && ogImage != "" {
		ogStatus = "pass"
	}
	ogDetails := fmt.Sprintf("OG title: %s, description: %s, image: %s.",
		yesNo(ogTitle != ""), yesNo(ogDesc != ""), yesNo(ogImage != ""))
	add("Social", "Open Graph Tags", ogStatus, ogDetails, "Add Open Graph title, description, image, URL, and type tags.", "Medium")

	twStatus := "warn"
	twDetails := "No Twitter/X card tag detected."
	if twitterCard != "" {
		twStatus = "pass"
		twDetails = "Twitter card found: " + twitterCard
	}
	add("Social", "Twitter/X Card", twStatus, twDetails, "Add Twitter/X card tags for richer social previews.", "Low")

	spStatus := "warn"
	spDetails := "No common social profile links detected."
	if len(socialProfiles) > 0 {
		spStatus = "pass"
		spDetails = "Profiles found: " + strings.Join(socialProfiles, ", ")
	}
	add("Social", "Social Profile Links", spStatus, spDetails, "Link to active Google, Facebook, Instagram, LinkedIn, YouTube, or Yelp profiles.", "Medium")

	// ── Local SEO ─────────────────────────────────────────────────────────
	if req.BusinessName != "" {
		bnStatus := "warn"
		if businessNamePresent {
			bnStatus = "pass"
		}
		bnDetails := fmt.Sprintf("Submitted business name was not found exactly on the page: %s", req.BusinessName)
		if businessNamePresent {
			bnDetails = "Business name appears on the page."
		}
		add("Local SEO", "Business Name Presence", bnStatus, bnDetails, "Use consistent business name across website, GBP, Yelp, Bing, Apple Maps, and directories.", "High")
	} else {
		add("Local SEO", "Business Name Presence", "info", "No business name was provided for NAP comparison.", "Use consistent business name across website, GBP, Yelp, Bing, Apple Maps, and directories.", "High")
	}

	napStatus := "warn"
	napDetails := "No phone number was detected on the page."
	if phonePresent {
		napStatus = "pass"
		napDetails = "Detected phone: " + strings.Join(phoneMatches[:seo.Min(3, len(phoneMatches))], ", ")
	}
	add("Local SEO", "Phone / NAP Presence", napStatus, napDetails, "Place the primary phone number in header, footer, contact page, and LocalBusiness schema.", "High")

	addrStatus := "warn"
	addrDetails := "No clear street address pattern detected."
	if addressHints {
		addrStatus = "pass"
		addrDetails = "Street/address-like text detected."
	}
	add("Local SEO", "Address Signal", addrStatus, addrDetails, "Add full NAP details if the business has a public address, or service-area details if not.", "High")

	if req.TargetLocation != "" {
		locStatus := "warn"
		locDetails := fmt.Sprintf("Target location not found exactly: %s", req.TargetLocation)
		if locationPresent {
			locStatus = "pass"
			locDetails = fmt.Sprintf("Target location appears: %s", req.TargetLocation)
		}
		add("Local SEO", "Location Keyword Usage", locStatus, locDetails, "Use city/service-area language naturally in title, headings, copy, and schema.", "High")
	} else {
		add("Local SEO", "Location Keyword Usage", "info", "No target location was provided.", "Use city/service-area language naturally in title, headings, copy, and schema.", "High")
	}

	mapStatus := "warn"
	if mapEmbed || req.GBPUrl != "" {
		mapStatus = "pass"
	}
	mapDetails := "No map embed or GBP link detected."
	if req.GBPUrl != "" {
		mapDetails = "GBP/Maps URL provided."
	} else if mapEmbed {
		mapDetails = "Map embed or map link detected."
	}
	add("Local SEO", "Google Map / GBP Signal", mapStatus, mapDetails, "Add Google Maps embed or link to the Google Business Profile on the contact/location page.", "Medium")

	lbsStatus := "warn"
	lbsDetails := "No structured data type detected."
	for _, t := range schemaTypes {
		if regexp.MustCompile(`(?i)LocalBusiness|ProfessionalService|HomeAndConstructionBusiness|Store|Restaurant`).MatchString(t) {
			lbsStatus = "pass"
			break
		}
	}
	if len(schemaTypes) > 0 {
		lbsDetails = "Schema types detected: " + strings.Join(schemaTypes, ", ")
	}
	add("Local SEO", "LocalBusiness Schema", lbsStatus, lbsDetails, "Add LocalBusiness schema with name, phone, address/service area, URL, hours, and sameAs profiles.", "High")

	// ── Technology ────────────────────────────────────────────────────────
	httpsStatus := "pass"
	if base.Scheme != "https" {
		httpsStatus = "fail"
	}
	httpsDetails := "Website uses HTTPS."
	if base.Scheme != "https" {
		httpsDetails = "Website is not using HTTPS."
	}
	add("Technology", "HTTPS Security", httpsStatus, httpsDetails, "Install SSL and redirect all HTTP traffic to HTTPS.", "High")

	mixedStatus := "pass"
	mixedDetails := "No obvious HTTP asset references found in homepage HTML."
	if base.Scheme == "https" && regexp.MustCompile(`http://`).MatchString(homeHTML) {
		mixedStatus = "warn"
		mixedDetails = "HTTP asset references found in source."
	}
	add("Technology", "Mixed Content Risk", mixedStatus, mixedDetails, "Update asset URLs to HTTPS and fix mixed content warnings.", "Medium")

	techStatus := "pass"
	if len(tech) == 0 {
		techStatus = "info"
	}
	techDetails := "No common CMS/technology signatures detected from source."
	if len(tech) > 0 {
		techDetails = "Detected: " + strings.Join(tech, ", ")
	}
	add("Technology", "Technology Detection", techStatus, techDetails, "Use a lightweight stack, keep plugins updated, and remove unused tools.", "Low")

	analyticsStatus := "warn"
	if regexp.MustCompile(`(?i)googletagmanager|google-analytics|gtag\(|analytics\.js|clarity\.ms|fbq\(`).MatchString(homeHTML) {
		analyticsStatus = "pass"
	}
	add("Technology", "Analytics / Tag Manager", analyticsStatus, "Marketing/analytics tag scan completed.", "Install GA4, Google Search Console, and conversion tracking for measurable growth.", "Medium")

	suspStatus := "pass"
	if suspicious.Risk == "medium" {
		suspStatus = "warn"
	} else if suspicious.Risk == "high" {
		suspStatus = "fail"
	}
	add("Technology", "Spam / Suspicious Link Risk", suspStatus, suspicious.Details, "Review suspicious outbound links, hidden links, and injected spam terms immediately.", "High")

	secStatus := "pass"
	if !sec.HSTS || !sec.XContentType || !sec.ReferrerPolicy {
		secStatus = "warn"
	}
	secDetails := fmt.Sprintf("HSTS: %s, CSP: %s, X-Content-Type-Options: %s, Referrer-Policy: %s.",
		yesNo(sec.HSTS), yesNo(sec.CSP), yesNo(sec.XContentType), yesNo(sec.ReferrerPolicy))
	add("Technology", "Security Headers", secStatus, secDetails, "Add HSTS, Content-Security-Policy, X-Content-Type-Options, Referrer-Policy, and Permissions-Policy headers where appropriate.", "Medium")

	trackingStatus := "warn"
	for _, t := range agg.Tech {
		if regexp.MustCompile(`(?i)GTM|GA4|Analytics`).MatchString(t) {
			trackingStatus = "pass"
			break
		}
	}
	trackingDetails := "No major tracking or CMS technology detected."
	if len(agg.Tech) > 0 {
		trackingDetails = "Site-wide technology detected: " + strings.Join(agg.Tech, ", ")
	}
	add("Technology", "Tracking & Conversion Stack", trackingStatus, trackingDetails, "Install GA4, Google Search Console, Google Tag Manager, pixels, and conversion tracking where appropriate.", "Medium")

	// ── Advanced modules ──────────────────────────────────────────────────
	psiMobileStatus := "info"
	psiMobileDetails := fmt.Sprintf("PageSpeed mobile not available: %s", psiM.Error)
	if psiM.Available {
		switch {
		case psiM.Performance >= 80:
			psiMobileStatus = "pass"
		case psiM.Performance >= 50:
			psiMobileStatus = "warn"
		default:
			psiMobileStatus = "fail"
		}
		psiMobileDetails = fmt.Sprintf("Mobile PageSpeed performance score: %d/100. LCP: %s, CLS: %s, INP: %s.", psiM.Performance, psiM.LCP, psiM.CLS, psiM.INP)
	}
	add("Performance", "Google PageSpeed Mobile", psiMobileStatus, psiMobileDetails, "Use PageSpeed opportunities to improve LCP, INP, CLS, image delivery, caching, and render-blocking assets.", "High")

	psiDesktopStatus := "info"
	psiDesktopDetails := fmt.Sprintf("PageSpeed desktop not available: %s", psiD.Error)
	if psiD.Available {
		switch {
		case psiD.Performance >= 85:
			psiDesktopStatus = "pass"
		case psiD.Performance >= 60:
			psiDesktopStatus = "warn"
		default:
			psiDesktopStatus = "fail"
		}
		psiDesktopDetails = fmt.Sprintf("Desktop PageSpeed performance score: %d/100. LCP: %s, CLS: %s.", psiD.Performance, psiD.LCP, psiD.CLS)
	}
	add("Performance", "Google PageSpeed Desktop", psiDesktopStatus, psiDesktopDetails, "Optimize images, caching, server response, CSS/JS delivery, and third-party scripts.", "High")

	mobileUsability := "pass"
	if viewport == "" {
		mobileUsability = "warn"
	}
	muDetails := fmt.Sprintf("Viewport: %s.", yesNo(viewport != ""))
	if psiM.Available {
		muDetails = fmt.Sprintf("Mobile accessibility score: %d/100. Viewport: %s.", psiM.Accessibility, yesNo(viewport != ""))
	}
	add("Usability", "Mobile Usability Signals", mobileUsability, muDetails, "Improve responsive layout, font sizing, tap targets, accessibility labels, and mobile performance.", "High")

	socialSitewideStatus := "warn"
	if len(agg.Social) >= 2 {
		socialSitewideStatus = "pass"
	}
	sSiteDetails := "No common social profile links were detected across scanned pages."
	if len(agg.Social) > 0 {
		sSiteDetails = "Social profiles found across sitemap pages: " + strings.Join(agg.Social, ", ")
	}
	add("Social", "Site-Wide Social Profiles", socialSitewideStatus, sSiteDetails, "Add consistent social links and sameAs schema for Google, Facebook, Instagram, LinkedIn, YouTube, Yelp, and other active profiles.", "Medium")

	blStatus := "pass"
	if bl.Broken > 0 {
		blStatus = "warn"
	}
	blDetails := fmt.Sprintf("%d link(s) checked from %d discovered link(s). Broken/blocked: %d.", bl.Checked, bl.TotalDiscovered, bl.Broken)
	if bl.Skipped != "" {
		blDetails = bl.Skipped
	}
	add("Links", "Broken Link Scan", blStatus, blDetails, "Fix or remove broken internal/external links, redirect outdated URLs, and update navigation links.", "High")

	extDomStatus := "pass"
	if len(agg.ExternalDomains) > 40 {
		extDomStatus = "warn"
	}
	add("Links", "External Domain Quality", extDomStatus,
		fmt.Sprintf("%d unique external domain(s) found site-wide.", len(agg.ExternalDomains)),
		"Review outbound domains and remove irrelevant, spammy, or outdated external links.", "Medium")

	gbpStatus := "info"
	gbpDetails := fmt.Sprintf("GBP/Maps API check not completed: %s", gp.Reason)
	if gp.Available {
		gbpStatus = "pass"
		gbpDetails = fmt.Sprintf("Google place found: %s • Rating: %.1f • Reviews: %d.", gp.Name, gp.Rating, gp.Reviews)
	}
	add("Local SEO", "Google Business Profile Data", gbpStatus, gbpDetails, "Connect Google Places/GBP access to verify listing match, rating, reviews, categories, phone, address, and website.", "High")

	yelpStatus := "info"
	yelpDetails := fmt.Sprintf("Yelp API check not completed: %s", yelp.Reason)
	if yelp.Available && len(yelp.Businesses) > 0 {
		yelpStatus = "pass"
		b := yelp.Businesses[0]
		yelpDetails = fmt.Sprintf("Yelp match found: %s • Rating: %.1f • Reviews: %d.", b.Name, b.Rating, b.Reviews)
	}
	add("Local SEO", "Yelp Listing Data", yelpStatus, yelpDetails, "Connect Yelp Fusion API or manually verify Yelp NAP consistency, reviews, category, and website URL.", "Medium")

	rankStatus := "info"
	rankDetails := fmt.Sprintf("Ranking data not completed: %s", rankings.Reason)
	if rankings.Available {
		if rankings.Matched {
			rankStatus = "pass"
			rankDetails = fmt.Sprintf("Domain found for %s at position %d.", rankings.Keyword, rankings.Position)
		} else {
			rankStatus = "warn"
			rankDetails = fmt.Sprintf("Domain not found in the checked top results for %s.", rankings.Keyword)
		}
	}
	add("Rankings", "Target Keyword Ranking", rankStatus, rankDetails, "Connect a SERP API and track target keywords by city, device, and date for visibility reporting.", "High")

	blinkStatus := "info"
	blinkDetails := fmt.Sprintf("Backlink data not completed: %s", backlinks.Reason)
	if backlinks.Available {
		if backlinks.ReferringDomains > 10 {
			blinkStatus = "pass"
		} else {
			blinkStatus = "warn"
		}
		blinkDetails = fmt.Sprintf("Backlinks: %d, referring domains: %d, broken backlinks: %d.", backlinks.Backlinks, backlinks.ReferringDomains, backlinks.BrokenBacklinks)
	}
	add("Authority", "Backlink / Domain Authority Data", blinkStatus, blinkDetails, "Build quality local citations, niche links, PR links, partner links, and fix broken backlinks.", "High")

	return checks
}

// ─────────────────────────────────────────────────────────────────────────────
// Response assembly
// ─────────────────────────────────────────────────────────────────────────────

func assembleResponse(
	id uuid.UUID, domain, websiteURL string, overallScore int, scores map[string]int,
	passed, warnings, failed []seo.Check,
	execSummary []string, actionPlan []seo.ActionPlanItem, listings []seo.ListingEntry,
	hp *seo.PageAnalysis, site *seo.SiteSummary,
	bl *seo.BrokenLinksResult, psiM, psiD *seo.PSIResult,
	agg *seo.AggregateSignals, sec *seo.SecurityHeaders,
	checks []seo.Check, ai seo.AIRecommendations,
) *model.SEOAuditResponse {

	// Action plan → model.ActionItem
	actionItems := make([]model.ActionItem, 0, len(actionPlan))
	for i, a := range actionPlan {
		state := "Warning"
		impact := "High Impact"
		desc := a.Description
		fix := ""

		// Find the matching check to get real details
		for _, c := range checks {
			if c.Title == a.Title {
				// Map status: pass -> Pass, warn -> Warning, fail -> Fail
				if c.Status == "pass" {
					state = "Pass"
				} else if c.Status == "fail" {
					state = "Fail"
				} else {
					state = "Warning"
				}

				// Map impact: High -> High Impact, etc.
				if c.Impact != "" {
					if strings.HasSuffix(strings.ToLower(c.Impact), "impact") {
						impact = c.Impact
					} else {
						impact = c.Impact + " Impact"
					}
				}

				desc = c.Details
				fix = c.Fix
				break
			}
		}

		actionItems = append(actionItems, model.ActionItem{
			ID:     fmt.Sprintf("action_%d", i+1),
			Title:  a.Title,
			Impact: impact,
			State:  state,
			Desc:   desc,
			Fix:    fix,
		})
	}

	// Page snapshot
	h1count := len(hp.H1s)
	h1tags := strings.Join(hp.H1s[:seo.Min(3, len(hp.H1s))], " | ")

	// Sitewide stats
	sw := model.SitewideStats{
		SitemapURLs:           len(site.SitemapURLs),
		PagesScanned:          site.PagesScanned,
		AvgPageScore:          site.AveragePageScore,
		FailedSkipped:         site.FailedPages,
		MissingTitles:         site.MissingTitles,
		MissingDescriptions:   site.MissingDescriptions,
		DuplicateTitles:       site.DuplicateTitles,
		DuplicateDescriptions: site.DuplicateDescriptions,
		MissingH1:             site.MissingH1,
		MultipleH1:            site.MultipleH1,
		ThinPages:             site.ThinPages,
		ImagesMissingAlt:      site.ImagesMissingAlt,
		TotalImages:           site.TotalImages,
		MissingCanonical:      site.MissingCanonical,
		PagesMissingSchema:    site.PagesMissingSchema,
	}

	// Page rows
	pageRows := make([]model.PageRow, 0, len(site.PageRows))
	for _, r := range site.PageRows {
		pageRows = append(pageRows, model.PageRow{URL: r.URL, Score: r.Score, Words: r.WordCount, Issues: r.Issues})
	}

	// Listings
	modelListings := make([]model.Listing, 0, len(listings))
	for _, l := range listings {
		modelListings = append(modelListings, model.Listing{Platform: l.Platform, Status: l.Status, Link: l.Link, Note: l.Note})
	}

	// Broken links
	brokenRows := make([]model.BrokenLink, 0, len(bl.Rows))
	for _, r := range bl.Rows {
		brokenRows = append(brokenRows, model.BrokenLink{URL: r.URL, Status: r.Status, Source: r.Source, Anchor: r.Anchor})
	}

	// PageSpeed
	psiMobileModel := model.PageSpeedResult{Available: psiM.Available, Score: psiM.Performance, Message: psiM.Error, LCP: psiM.LCP, CLS: psiM.CLS, INP: psiM.INP}
	psiDesktopModel := model.PageSpeedResult{Available: psiD.Available, Score: psiD.Performance, Message: psiD.Error, LCP: psiD.LCP, CLS: psiD.CLS}

	// Security headers
	secModel := model.SecurityHeaders{
		HSTS: sec.HSTS, CSP: sec.CSP, XFrameOptions: sec.XFrameOptions,
		XContentType: sec.XContentType, ReferrerPolicy: sec.ReferrerPolicy,
		PermissionsPolicy: sec.PermissionsPolicy, Server: sec.Server, PoweredBy: sec.PoweredBy,
	}

	// Breakdown map: rename "Local SEO" key for frontend
	breakdown := make(map[string]int, len(scores))
	for k, v := range scores {
		key := strings.ReplaceAll(k, " ", "")
		breakdown[key] = v
	}

	// Category checks
	checksByCategory := make(map[string][]seo.Check)
	for _, c := range checks {
		checksByCategory[c.Category] = append(checksByCategory[c.Category], c)
	}
	toModelChecks := func(cat string) []model.Check {
		var out []model.Check
		for _, c := range checksByCategory[cat] {
			out = append(out, model.Check{Status: c.Status, Title: c.Title, Impact: c.Impact, Details: c.Details, Fix: c.Fix})
		}
		return out
	}

	aiStatus := model.AIStatus{Available: ai.Available, Source: ai.Source, Reason: ai.Reason}

	return &model.SEOAuditResponse{
		ID:           id,
		Domain:       domain,
		WebsiteURL:   websiteURL,
		OverallScore: overallScore,
		OverallGrade: seo.Grade(overallScore),
		GeneratedAt:  time.Now().UTC(),
		MetricsSummary: model.MetricsSummary{
			Passed:          len(passed),
			Warnings:        len(warnings),
			Failed:          len(failed),
			SitemapsScanned: len(site.SitemapURLs),
			URLsDiscovered:  site.SitemapPageCount,
		},
		Breakdown:        breakdown,
		ExecutiveSummary: execSummary,
		ActionPlan:       actionItems,
		PageSnapshot: model.PageSnapshot{
			Title:             hp.Title,
			TitleLength:       len(hp.Title),
			Description:       hp.Description,
			DescriptionLength: len(hp.Description),
			H1Count:           h1count,
			H1Tags:            h1tags,
			WordCount:         hp.WordCount,
		},
		Sitewide: sw,
		PageRows: pageRows,
		Listings: modelListings,
		Advanced: model.AdvancedData{
			PageSpeed:       model.PageSpeedData{Mobile: psiMobileModel, Desktop: psiDesktopModel},
			BrokenLinks:     brokenRows,
			BrokenChecked:   bl.Checked,
			BrokenTotal:     bl.TotalDiscovered,
			Technology:      agg.Tech,
			SocialProfiles:  agg.Social,
			SecurityHeaders: secModel,
		},
		Checks: model.CategoryChecks{
			SEO:         toModelChecks("SEO"),
			Usability:   toModelChecks("Usability"),
			Performance: toModelChecks("Performance"),
			Social:      toModelChecks("Social"),
			Local:       toModelChecks("Local SEO"),
			Tech:        toModelChecks("Technology"),
			Links:       toModelChecks("Links"),
			Rankings:    toModelChecks("Rankings"),
			Authority:   toModelChecks("Authority"),
		},
		AI: aiStatus,
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func errSuffix(e string) string {
	if e != "" {
		return " - " + e
	}
	return ""
}
