package seo

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

type AuditRequest struct {
	WebsiteURL     string
	BusinessName   string
	TargetKeyword  string
	TargetLocation string
	Phone          string
	GBPUrl         string
}

type PageAnalysis struct {
	OK                   bool
	Status               int
	URL                  string
	FinalURL             string
	Error                string
	Title                string
	Description          string
	Canonical            string
	RobotsMeta           string
	Viewport             string
	Lang                 string
	H1s                  []string
	H2s                  []string
	H3s                  []string
	WordCount            int
	Images               int
	ImagesMissingAlt     int
	GenericAlt           int
	LazyImages           int
	BadFileImages        int
	InternalLinks        int
	ExternalLinks        int
	UniqueExternalDomains []string
	Scripts              int
	Stylesheets          int
	ThirdPartyScripts    int
	Forms                int
	SchemaTypes          []string
	OGTitle              string
	OGDesc               string
	OGImage              string
	TwitterCard          string
	Favicon              string
	Generator            string
	Tech                 []string
	SocialProfiles       []string
	PhoneMatches         []string
	EmailMatches         []string
	AddressHints         bool
	MapEmbed             bool
	BusinessNamePresent  bool
	KeywordPresent       bool
	LocationPresent      bool
	PhonePresent         bool
	Suspicious           SuspiciousResult
	HTMLSizeKB           int
	PageText             string
	Links                []Link
	Score                int
	Issues               []string
}

// ─────────────────────────────────────────────────────────────────────────────
// HTML parsing helpers
// ─────────────────────────────────────────────────────────────────────────────

type parsedDoc struct {
	node *html.Node
}

func parseHTML(text string) (*parsedDoc, error) {
	doc, err := html.Parse(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	return &parsedDoc{node: doc}, nil
}

func (d *parsedDoc) queryAll(tag string) []*html.Node {
	var results []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, tag) {
			results = append(results, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(d.node)
	return results
}

func (d *parsedDoc) queryFirst(tag string) *html.Node {
	nodes := d.queryAll(tag)
	if len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

func visibleText(doc *parsedDoc) string {
	skipTags := map[string]bool{"script": true, "style": true, "noscript": true, "svg": true, "canvas": true, "iframe": true}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[strings.ToLower(n.Data)] {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc.node)
	return strings.Join(strings.Fields(sb.String()), " ")
}

func getMeta(doc *parsedDoc, name, prop string) string {
	for _, n := range doc.queryAll("meta") {
		if (name != "" && strings.EqualFold(attr(n, "name"), name)) ||
			(prop != "" && strings.EqualFold(attr(n, "property"), prop)) {
			return strings.TrimSpace(attr(n, "content"))
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Main page analyzer
// ─────────────────────────────────────────────────────────────────────────────

// AnalyzeHTMLPage parses a fetched HTML page and produces a PageAnalysis.
// Ported from extension's analyzeHtmlPage().
func AnalyzeHTMLPage(pageURL string, response FetchResult, request AuditRequest, rootURL string) PageAnalysis {
	result := PageAnalysis{
		OK:       response.OK,
		Status:   response.Status,
		URL:      pageURL,
		FinalURL: response.URL,
	}
	if !response.OK || response.Text == "" {
		result.Error = response.Error
		return result
	}

	doc, err := parseHTML(response.Text)
	if err != nil {
		result.Error = "HTML parse error: " + err.Error()
		return result
	}

	pageText := visibleText(doc)
	wc := WordCount(pageText)

	// Title
	titleNode := doc.queryFirst("title")
	title := textContent(titleNode)

	// Meta tags
	description := getMeta(doc, "description", "description")
	robotsMeta := getMeta(doc, "robots", "")
	viewport := getMeta(doc, "viewport", "")
	generator := getMeta(doc, "generator", "")
	ogTitle := getMeta(doc, "", "og:title")
	ogDesc := getMeta(doc, "", "og:description")
	ogImage := getMeta(doc, "", "og:image")
	twitterCard := getMeta(doc, "twitter:card", "")

	// Canonical
	var canonical string
	for _, link := range doc.queryAll("link") {
		if strings.EqualFold(attr(link, "rel"), "canonical") {
			canonical = attr(link, "href")
			break
		}
	}

	// Lang
	var lang string
	for _, n := range doc.queryAll("html") {
		lang = attr(n, "lang")
		break
	}

	// Favicon
	var favicon string
	for _, link := range doc.queryAll("link") {
		rel := strings.ToLower(attr(link, "rel"))
		if rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon" {
			favicon = attr(link, "href")
			break
		}
	}

	// Headings
	var h1s, h2s, h3s []string
	for _, n := range doc.queryAll("h1") {
		if t := textContent(n); t != "" {
			h1s = append(h1s, t)
		}
	}
	for _, n := range doc.queryAll("h2") {
		if t := textContent(n); t != "" {
			h2s = append(h2s, t)
		}
	}
	for _, n := range doc.queryAll("h3") {
		if t := textContent(n); t != "" {
			h3s = append(h3s, t)
		}
	}

	// Images
	imgNodes := doc.queryAll("img")
	imagesMissingAlt := 0
	genericAlt := 0
	lazyImages := 0
	badFileImages := 0
	genericAltRe := regexp.MustCompile(`(?i)^(image|photo|picture|logo|banner|img)$`)
	badFileRe := regexp.MustCompile(`(?i)(img_|dsc_|screenshot|untitled|image\d|photo\d|pxl_|whatsapp)`)
	for _, img := range imgNodes {
		a := strings.TrimSpace(attr(img, "alt"))
		if a == "" {
			imagesMissingAlt++
		} else if genericAltRe.MatchString(a) {
			genericAlt++
		}
		if strings.ToLower(attr(img, "loading")) == "lazy" {
			lazyImages++
		}
		if badFileRe.MatchString(attr(img, "src")) {
			badFileImages++
		}
	}

	// Links
	var links []Link
	for _, a := range doc.queryAll("a") {
		href := attr(a, "href")
		if href == "" {
			continue
		}
		abs := AbsoluteURL(href, response.URL)
		if abs == "" || (!strings.HasPrefix(abs, "http://") && !strings.HasPrefix(abs, "https://")) {
			continue
		}
		links = append(links, Link{Href: abs, Text: textContent(a)})
	}

	var internalLinks, externalLinks []Link
	extDomainSet := make(map[string]bool)
	for _, l := range links {
		if SameRootHost(l.Href, rootURL) {
			internalLinks = append(internalLinks, l)
		} else {
			externalLinks = append(externalLinks, l)
			extDomainSet[DomainFromURL(l.Href)] = true
		}
	}
	uniqueExternalDomains := make([]string, 0, len(extDomainSet))
	for d := range extDomainSet {
		uniqueExternalDomains = append(uniqueExternalDomains, d)
	}

	// Scripts & stylesheets
	scripts := doc.queryAll("script")
	stylesheets := doc.queryAll("link")
	thirdPartyScripts := 0
	for _, s := range scripts {
		src := attr(s, "src")
		if src == "" {
			continue
		}
		abs := AbsoluteURL(src, response.URL)
		if abs != "" && !SameRootHost(abs, rootURL) {
			thirdPartyScripts++
		}
	}
	stylesheetCount := 0
	for _, l := range stylesheets {
		if strings.EqualFold(attr(l, "rel"), "stylesheet") {
			stylesheetCount++
		}
	}

	forms := doc.queryAll("form")

	// Schema
	var schemaBuf strings.Builder
	for _, s := range doc.queryAll("script") {
		if strings.EqualFold(attr(s, "type"), "application/ld+json") {
			schemaBuf.WriteString(textContent(s))
			schemaBuf.WriteString("\n")
		}
	}
	schemaTypes := DetectSchemaTypes(schemaBuf.String())

	// Tech & social
	tech := DetectTech(response.Text, generator)
	socialProfiles := DetectSocialProfiles(links)

	// Contact signals
	phoneRe := regexp.MustCompile(`(?:\+?1[\s.-]?)?\(?\d{3}\)?[\s.-]?\d{3}[\s.-]?\d{4}`)
	phoneMatches := uniqueStrings(phoneRe.FindAllString(pageText, -1))

	emailRe := regexp.MustCompile(`[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}`)
	emailMatches := uniqueStrings(emailRe.FindAllString(strings.ToUpper(pageText), -1))
	for i := range emailMatches {
		emailMatches[i] = strings.ToLower(emailMatches[i])
	}

	addressRe := regexp.MustCompile(`(?i)\b\d{1,6}\s+[A-Za-z0-9.' -]+\s+(Street|St\.?|Avenue|Ave\.?|Road|Rd\.?|Boulevard|Blvd\.?|Drive|Dr\.?|Lane|Ln\.?|Court|Ct\.?|Way|Suite|Ste\.?|Unit)\b`)
	addressHints := addressRe.MatchString(pageText)

	mapRe := regexp.MustCompile(`(?i)google\.com/maps|maps\.app\.goo\.gl|bing\.com/maps|apple\.com/maps`)
	mapEmbed := mapRe.MatchString(response.Text)

	businessNamePresent := request.BusinessName != "" && strings.Contains(strings.ToLower(pageText), strings.ToLower(request.BusinessName))
	keywordPresent := request.TargetKeyword != "" && strings.Contains(strings.ToLower(pageText), strings.ToLower(request.TargetKeyword))
	locationPresent := request.TargetLocation != "" && strings.Contains(strings.ToLower(pageText), strings.ToLower(request.TargetLocation))

	phonePresent := false
	if request.Phone != "" {
		phonePresent = strings.Contains(NormalizePhone(pageText), NormalizePhone(request.Phone))
	} else {
		phonePresent = len(phoneMatches) > 0
	}

	suspicious := DetectSuspiciousSignals(response.Text, links)
	htmlSizeKB := len(response.Text) / 1024
	noindex := regexp.MustCompile(`(?i)noindex`).MatchString(robotsMeta)

	pageScore := ComputePageSEOScore(PageScoreParams{
		Title:            title,
		Description:      description,
		H1s:              h1s,
		H2s:              h2s,
		WordCount:        wc,
		TotalImages:      len(imgNodes),
		ImagesMissingAlt: imagesMissingAlt,
		Canonical:        canonical,
		SchemaTypes:      schemaTypes,
		Noindex:          noindex,
		InternalLinks:    len(internalLinks),
	})

	var issues []string
	if title == "" {
		issues = append(issues, "Missing title")
	} else if len(title) < 30 || len(title) > 65 {
		issues = append(issues, "Title length")
	}
	if description == "" {
		issues = append(issues, "Missing meta description")
	} else if len(description) < 70 || len(description) > 165 {
		issues = append(issues, "Meta length")
	}
	if len(h1s) == 0 {
		issues = append(issues, "Missing H1")
	} else if len(h1s) > 1 {
		issues = append(issues, "Multiple H1")
	}
	if wc < 350 {
		issues = append(issues, "Thin content")
	}
	if imagesMissingAlt > 0 {
		issues = append(issues, "images missing alt")
	}
	if canonical == "" {
		issues = append(issues, "Missing canonical")
	}
	if len(schemaTypes) == 0 {
		issues = append(issues, "No schema")
	}
	if noindex {
		issues = append(issues, "Noindex")
	}

	result.Title = title
	result.Description = description
	result.Canonical = canonical
	result.RobotsMeta = robotsMeta
	result.Viewport = viewport
	result.Lang = lang
	result.H1s = h1s
	result.H2s = h2s
	result.H3s = h3s
	result.WordCount = wc
	result.Images = len(imgNodes)
	result.ImagesMissingAlt = imagesMissingAlt
	result.GenericAlt = genericAlt
	result.LazyImages = lazyImages
	result.BadFileImages = badFileImages
	result.InternalLinks = len(internalLinks)
	result.ExternalLinks = len(externalLinks)
	result.UniqueExternalDomains = uniqueExternalDomains
	result.Scripts = len(scripts)
	result.Stylesheets = stylesheetCount
	result.ThirdPartyScripts = thirdPartyScripts
	result.Forms = len(forms)
	result.SchemaTypes = schemaTypes
	result.OGTitle = ogTitle
	result.OGDesc = ogDesc
	result.OGImage = ogImage
	result.TwitterCard = twitterCard
	result.Favicon = favicon
	result.Generator = generator
	result.Tech = tech
	result.SocialProfiles = socialProfiles
	result.PhoneMatches = phoneMatches
	result.EmailMatches = emailMatches
	result.AddressHints = addressHints
	result.MapEmbed = mapEmbed
	result.BusinessNamePresent = businessNamePresent
	result.KeywordPresent = keywordPresent
	result.LocationPresent = locationPresent
	result.PhonePresent = phonePresent
	result.Suspicious = suspicious
	result.HTMLSizeKB = htmlSizeKB
	result.PageText = pageText
	result.Links = links
	result.Score = pageScore
	result.Issues = issues
	return result
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range items {
		s = strings.TrimSpace(s)
		if !seen[s] && s != "" {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
