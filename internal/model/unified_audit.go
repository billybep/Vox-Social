package model

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// SPRINT 2 WORKFLOW — UNIFIED AUDIT MODELS
// ─────────────────────────────────────────────────────────────────────────────

// CompetitorInput represents a competitor configured in the Create Audit Wizard.
type CompetitorInput struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
	Active bool   `json:"active"`
}

// AuditPillarsConfig specifies which analysis pillars are enabled.
type AuditPillarsConfig struct {
	SEO    bool `json:"seo"`
	Social bool `json:"social"`
	AI     bool `json:"ai"`
	CRO    bool `json:"cro"`
}

// AuditLanguagesConfig specifies language coverage (e.g. EN, AR).
type AuditLanguagesConfig struct {
	EN bool `json:"en"`
	AR bool `json:"ar"`
}

// AuditRunRequest is the payload sent by CreateAuditWizard (POST /api/v1/audits/run).
type AuditRunRequest struct {
	TargetDomain    string               `json:"targetDomain"`
	BrandName       string               `json:"brandName"`
	Pillars         AuditPillarsConfig   `json:"pillars"`
	Languages       AuditLanguagesConfig `json:"languages"`
	Competitors     []CompetitorInput    `json:"competitors"`
	CreditsDeducted int                  `json:"creditsDeducted"`
}

// AuditRunResponse is returned when an audit is started.
type AuditRunResponse struct {
	AuditID          string    `json:"auditId"`
	Status           string    `json:"status"` // "running"
	CreditsRemaining int       `json:"creditsRemaining"`
	StartedAt        time.Time `json:"startedAt"`
}

// AuditStage represents one discrete progress step in the AuditProgressCenter.
type AuditStage struct {
	ID        string  `json:"id"`        // "stage-1" through "stage-5"
	Name      string  `json:"name"`      // "Website Crawl & VoxSEO", etc.
	Status    string  `json:"status"`    // "completed" | "running" | "pending" | "failed"
	Timestamp *string `json:"timestamp"` // elapsed time string e.g. "00:04", or nil
	Details   string  `json:"details"`   // "51 checkpoints verified", etc.
}

// AuditStatusResponse is returned by GET /api/v1/audits/:id/status.
type AuditStatusResponse struct {
	AuditID       string       `json:"auditId"`
	OverallStatus string       `json:"overallStatus"` // "running" | "partial_failed" | "completed"
	Stages        []AuditStage `json:"stages"`
}

// RetrySourceRequest is the payload for POST /api/v1/audits/:id/retry-source.
type RetrySourceRequest struct {
	StageID string `json:"stageId"`
}

// CompetitorBenchmark represents comparative score against a competitor.
type CompetitorBenchmark struct {
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	OverallScore int    `json:"overallScore"`
	SEOScore     int    `json:"seoScore"`
	SocialScore  int    `json:"socialScore"`
	IsClient     bool   `json:"isClient"`
	IsEstimated  bool   `json:"isEstimated"`
}

// UnifiedFinding represents a key discovery across any of the 4 pillars.
type UnifiedFinding struct {
	ID          string `json:"id"`
	Pillar      string `json:"pillar"`      // "SEO" | "Social" | "AI" | "CRO"
	Severity    string `json:"severity"`    // "critical" | "watch" | "good"
	Title       string `json:"title"`
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
	Source      string `json:"source"`      // "Measured" | "Connected" | "Estimated" | "AI interpretation"
}

// RoadmapItem represents a recommended action in the 90-day growth plan.
type RoadmapItem struct {
	ID        string `json:"id"`
	Pillar    string `json:"pillar"`
	Timeframe string `json:"timeframe"` // "30-day" | "60-day" | "90-day"
	Title     string `json:"title"`
	Impact    string `json:"impact"`    // "High" | "Medium" | "Low"
	Effort    string `json:"effort"`    // "Low" | "Medium" | "High"
}

// UnifiedAuditResult is returned by GET /api/v1/audits/:id/results.
type UnifiedAuditResult struct {
	AuditID          string                `json:"auditId"`
	TargetDomain     string                `json:"targetDomain"`
	BrandName        string                `json:"brandName"`
	OverallStatus    string                `json:"overallStatus"`
	StartedAt        time.Time             `json:"startedAt"`
	CompletedAt      *time.Time            `json:"completedAt,omitempty"`
	OverallScore     int                   `json:"overallScore"`
	OverallGrade     string                `json:"overallGrade"`
	PillarsScores    map[string]int        `json:"pillarsScores"`
	SEOScore         int                   `json:"seoScore"`
	SocialScore      int                   `json:"socialScore"`
	AIScore          int                   `json:"aiScore"`
	CROScore         int                   `json:"croScore"`
	Competitors      []CompetitorBenchmark `json:"competitors"`
	KeyFindings      []UnifiedFinding      `json:"keyFindings"`
	RoadmapItems     []RoadmapItem         `json:"roadmapItems"`
	ExecutiveSummary string                `json:"executiveSummary"`
}
