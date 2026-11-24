package model

import (
	"encoding/json"
	"time"
)

// AuditRecord represents a persistent audit record in the audit_records table.
type AuditRecord struct {
	ID               string          `json:"id"`
	UserID           string          `json:"user_id"`
	AuditType        string          `json:"audit_type"` // 'seo', 'social', 'unified'
	TargetDomain     string          `json:"target_domain"`
	BrandName        string          `json:"brand_name"`
	SocialAccount    string          `json:"social_account,omitempty"`
	OverallScore     int             `json:"overall_score"`
	SEOScore         *int            `json:"seo_score"`
	SocialScore      *int            `json:"social_score"`
	Status           string          `json:"status"` // 'completed', 'failed', 'running'
	EvidenceCoverage int             `json:"evidence_coverage"`
	FullData         json.RawMessage `json:"full_data,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// AuditRecordSummary represents a lightweight record for lists and histories without heavy full_data payloads.
type AuditRecordSummary struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	AuditType        string    `json:"audit_type"`
	TargetDomain     string    `json:"target_domain"`
	BrandName        string    `json:"brand_name"`
	SocialAccount    string    `json:"social_account,omitempty"`
	OverallScore     int       `json:"overall_score"`
	SEOScore         *int      `json:"seo_score"`
	SocialScore      *int      `json:"social_score"`
	Status           string    `json:"status"`
	EvidenceCoverage int       `json:"evidence_coverage"`
	CreatedAt        time.Time `json:"created_at"`
}

// AuditHistoryResponse represents the JSON response for GET /api/v1/audits/history.
type AuditHistoryResponse struct {
	Success bool                 `json:"success"`
	Data    []AuditRecordSummary `json:"data"`
	Total   int                  `json:"total"`
}

// AuditLatestResponse represents the JSON response for GET /api/v1/audits/latest.
type AuditLatestResponse struct {
	Success bool         `json:"success"`
	Data    *AuditRecord `json:"data"`
}

// AuditDetailResponse represents the JSON response for GET /api/v1/audits/:id.
type AuditDetailResponse struct {
	Success bool         `json:"success"`
	Data    *AuditRecord `json:"data"`
	Error   string       `json:"error,omitempty"`
}
