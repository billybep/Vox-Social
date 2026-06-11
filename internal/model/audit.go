package model

import (
	"time"

	"github.com/google/uuid"
)

// AnalyzeRequest represents the incoming JSON payload for /api/v1/analyze
type AnalyzeRequest struct {
	ProfileURL string `json:"profile_url"`
}

// AnalyzeResponse represents the expected JSON response
type AnalyzeResponse struct {
	OverallScore     int      `json:"overall_score"`
	Status           string   `json:"status"`
	ProfileIdentity  string   `json:"profile_identity"`
	GrowthPotential  string   `json:"growth_potential"`
	ProfileReadiness int      `json:"profile_readiness"`
	KeyStrengths     []string `json:"key_strengths"`
	Opportunities    []string `json:"opportunities"`
}

// SocialAudit represents the database record structure
type SocialAudit struct {
	ID               uuid.UUID `json:"id"`
	TargetURL        string    `json:"target_url"`
	OverallScore     int       `json:"overall_score"`
	Status           string    `json:"status"`
	ProfileIdentity  string    `json:"profile_identity"`
	GrowthPotential  string    `json:"growth_potential"`
	ProfileReadiness int       `json:"profile_readiness"`
	KeyStrengths     []string  `json:"key_strengths"`
	Opportunities    []string  `json:"opportunities"`
	CheckedAt        time.Time `json:"checked_at"`
}
