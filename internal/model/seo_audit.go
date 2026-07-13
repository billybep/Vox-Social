package model

import (
	"time"

	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// REQUEST
// ─────────────────────────────────────────────────────────────────────────────

// SEOAuditRequest is the JSON body accepted by POST /api/v1/seo/audit.
type SEOAuditRequest struct {
	Domain         string `json:"domain"`
	BusinessName   string `json:"business_name,omitempty"`
	TargetKeyword  string `json:"target_keyword,omitempty"`
	TargetLocation string `json:"target_location,omitempty"`
	Phone          string `json:"phone,omitempty"`
	GBPUrl         string `json:"gbp_url,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// RESPONSE — top-level
// ─────────────────────────────────────────────────────────────────────────────

type SEOAuditResponse struct {
	ID               uuid.UUID          `json:"id"`
	Domain           string             `json:"domain"`
	WebsiteURL       string             `json:"website_url"`
	OverallScore     int                `json:"overall_score"`
	OverallGrade     string             `json:"overall_grade"`
	GeneratedAt      time.Time          `json:"generated_at"`
	MetricsSummary   MetricsSummary     `json:"metrics_summary"`
	Breakdown        map[string]int     `json:"breakdown"`
	ExecutiveSummary []string           `json:"executive_summary"`
	ActionPlan       []ActionItem       `json:"action_plan"`
	PageSnapshot     PageSnapshot       `json:"page_snapshot"`
	Sitewide         SitewideStats      `json:"sitewide"`
	PageRows         []PageRow          `json:"page_rows"`
	Listings         []Listing          `json:"listings"`
	Advanced         AdvancedData       `json:"advanced"`
	Checks           CategoryChecks     `json:"checks"`
	AI               AIStatus           `json:"ai"`
}

// ─────────────────────────────────────────────────────────────────────────────
// NESTED TYPES
// ─────────────────────────────────────────────────────────────────────────────

type MetricsSummary struct {
	Passed          int `json:"passed"`
	Warnings        int `json:"warnings"`
	Failed          int `json:"failed"`
	SitemapsScanned int `json:"sitemaps_scanned"`
	URLsDiscovered  int `json:"urls_discovered"`
}

type ActionItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Impact string `json:"impact"`
	State  string `json:"state"`
	Desc   string `json:"desc"`
	Fix    string `json:"fix"`
}

type PageSnapshot struct {
	Title             string `json:"title"`
	TitleLength       int    `json:"title_length"`
	Description       string `json:"description"`
	DescriptionLength int    `json:"description_length"`
	H1Count           int    `json:"h1_count"`
	H1Tags            string `json:"h1_tags"` // first 3 joined
	WordCount         int    `json:"word_count"`
}

type SitewideStats struct {
	SitemapURLs          int `json:"sitemap_urls"`
	PagesScanned         int `json:"pages_scanned"`
	AvgPageScore         int `json:"avg_page_score"`
	FailedSkipped        int `json:"failed_skipped"`
	MissingTitles        int `json:"missing_titles"`
	MissingDescriptions  int `json:"missing_descriptions"`
	DuplicateTitles      int `json:"duplicate_titles"`
	DuplicateDescriptions int `json:"duplicate_descriptions"`
	MissingH1            int `json:"missing_h1"`
	MultipleH1           int `json:"multiple_h1"`
	ThinPages            int `json:"thin_pages"`
	ImagesMissingAlt     int `json:"images_missing_alt"`
	TotalImages          int `json:"total_images"`
	MissingCanonical     int `json:"missing_canonical"`
	PagesMissingSchema   int `json:"pages_missing_schema"`
}

type PageRow struct {
	URL       string `json:"url"`
	Score     int    `json:"score"`
	Words     int    `json:"words"`
	Issues    string `json:"issues"`
}

type Listing struct {
	Platform string `json:"platform"`
	Status   string `json:"status"`
	Link     string `json:"link"`
	Note     string `json:"note"`
}

type AdvancedData struct {
	PageSpeed       PageSpeedData     `json:"pagespeed"`
	BrokenLinks     []BrokenLink      `json:"broken_links"`
	BrokenChecked   int               `json:"broken_links_checked"`
	BrokenTotal     int               `json:"broken_links_total"`
	Technology      []string          `json:"technology"`
	SocialProfiles  []string          `json:"social_profiles"`
	SecurityHeaders SecurityHeaders   `json:"security_headers"`
}

type PageSpeedData struct {
	Mobile  PageSpeedResult `json:"mobile"`
	Desktop PageSpeedResult `json:"desktop"`
}

type PageSpeedResult struct {
	Available   bool   `json:"available"`
	Score       int    `json:"score"`
	SEO         *int   `json:"seo,omitempty"`
	Accessibility *int `json:"accessibility,omitempty"`
	Message     string `json:"message"`
	LCP         string `json:"lcp,omitempty"`
	CLS         string `json:"cls,omitempty"`
	INP         string `json:"inp,omitempty"`
	FCP         string `json:"fcp,omitempty"`
	SpeedIndex  string `json:"speed_index,omitempty"`
	TBT         string `json:"tbt,omitempty"`
}

type BrokenLink struct {
	URL    string `json:"url"`
	Status string `json:"status"`
	Source string `json:"source"`
	Anchor string `json:"anchor,omitempty"`
}

type SecurityHeaders struct {
	HSTS              bool   `json:"hsts"`
	CSP               bool   `json:"csp"`
	XFrameOptions     bool   `json:"x_frame_options"`
	XContentType      bool   `json:"x_content_type"`
	ReferrerPolicy    bool   `json:"referrer_policy"`
	PermissionsPolicy bool   `json:"permissions_policy"`
	Server            string `json:"server,omitempty"`
	PoweredBy         string `json:"powered_by,omitempty"`
}

type Check struct {
	Status  string `json:"status"`  // pass | warn | fail | info
	Title   string `json:"title"`
	Impact  string `json:"impact"`
	Details string `json:"details"`
	Fix     string `json:"fix"`
}

type CategoryChecks struct {
	SEO         []Check `json:"seo"`
	Usability   []Check `json:"usability"`
	Performance []Check `json:"performance"`
	Social      []Check `json:"social"`
	Local       []Check `json:"local"`
	Tech        []Check `json:"tech"`
	Links       []Check `json:"links"`
	Rankings    []Check `json:"rankings"`
	Authority   []Check `json:"authority"`
}

type AIStatus struct {
	Available bool   `json:"available"`
	Source    string `json:"source,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// DATABASE RECORD
// ─────────────────────────────────────────────────────────────────────────────

// SEOAuditRecord is stored in the seo_audits PostgreSQL table.
type SEOAuditRecord struct {
	ID               uuid.UUID `json:"id"`
	Domain           string    `json:"domain"`
	WebsiteURL       string    `json:"website_url"`
	OverallScore     int       `json:"overall_score"`
	OverallGrade     string    `json:"overall_grade"`
	PassedCount      int       `json:"passed_count"`
	WarningCount     int       `json:"warning_count"`
	FailedCount      int       `json:"failed_count"`
	AIUsed           bool      `json:"ai_used"`
	GeneratedAt      time.Time `json:"generated_at"`
}

// SEOAuditSummary is returned for domain history lists.
type SEOAuditSummary struct {
	ID           uuid.UUID `json:"id"`
	OverallScore int       `json:"overall_score"`
	OverallGrade string    `json:"overall_grade"`
	GeneratedAt  time.Time `json:"generated_at"`
}
