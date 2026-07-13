package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type StorageService struct {
	pool *pgxpool.Pool
}

func NewStorageService(ctx context.Context, dbURL string) (*StorageService, error) {
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database url: %w", err)
	}

	// Optimize for Railway and low memory footprints
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return &StorageService{pool: pool}, nil
}

func (s *StorageService) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// SaveAuditAsync saves the audit result to PostgreSQL asynchronously.
func (s *StorageService) SaveAuditAsync(targetURL string, result *model.AnalyzeResponse) {
	// Run in a background goroutine
	go func() {
		// Use a standalone context for the background task to ensure it's not canceled when HTTP request finishes
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		query := `
			INSERT INTO social_audits (
				id, target_url, overall_score, status, profile_identity, 
				growth_potential, profile_readiness, key_strengths, opportunities, recommended_package, checked_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
		`
		
		newID := uuid.New()
		_, err := s.pool.Exec(ctx, query,
			newID,
			targetURL,
			result.OverallScore,
			result.Status,
			result.ProfileIdentity,
			result.GrowthPotential,
			result.ProfileReadiness,
			result.KeyStrengths,
			result.Opportunities,
			result.RecommendedPackage,
			time.Now(),
		)

		if err != nil {
			log.Printf("ERROR: Failed to save audit async for %s: %v\n", targetURL, err)
			return
		}
		
		log.Printf("Successfully saved async audit for URL: %s with ID: %s\n", targetURL, newID.String())
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// SEO Audit Storage
// ─────────────────────────────────────────────────────────────────────────────

// SaveSEOAudit persists a completed SEO audit to the seo_audits table (async).
func (s *StorageService) SaveSEOAudit(resp *model.SEOAuditResponse) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		toJSON := func(v interface{}) []byte {
			b, _ := json.Marshal(v)
			return b
		}

		query := `
			INSERT INTO seo_audits (
				id, domain, website_url, overall_score, overall_grade,
				passed_count, warning_count, failed_count, ai_used,
				scores, checks, executive_summary, action_plan,
				page_snapshot, sitewide, page_rows, listings, advanced,
				request_meta, generated_at
			) VALUES (
				$1,$2,$3,$4,$5,
				$6,$7,$8,$9,
				$10,$11,$12,$13,
				$14,$15,$16,$17,$18,
				$19,$20
			)
		`
		requestMeta, _ := json.Marshal(map[string]interface{}{})

		_, err := s.pool.Exec(ctx, query,
			resp.ID,
			resp.Domain,
			resp.WebsiteURL,
			resp.OverallScore,
			resp.OverallGrade,
			resp.MetricsSummary.Passed,
			resp.MetricsSummary.Warnings,
			resp.MetricsSummary.Failed,
			resp.AI.Available,
			toJSON(resp.Breakdown),
			toJSON(resp.Checks),
			toJSON(resp.ExecutiveSummary),
			toJSON(resp.ActionPlan),
			toJSON(resp.PageSnapshot),
			toJSON(resp.Sitewide),
			toJSON(resp.PageRows),
			toJSON(resp.Listings),
			toJSON(resp.Advanced),
			requestMeta,
			resp.GeneratedAt,
		)
		if err != nil {
			log.Printf("[SEO] ERROR saving SEO audit for %s: %v", resp.Domain, err)
			return
		}
		log.Printf("[SEO] Saved SEO audit %s for domain: %s", resp.ID, resp.Domain)
	}()
}

// GetSEOAuditByID loads a full SEO audit report from the database by UUID.
func (s *StorageService) GetSEOAuditByID(ctx context.Context, id string) (*model.SEOAuditResponse, error) {
	query := `
		SELECT id, domain, website_url, overall_score, overall_grade,
		       passed_count, warning_count, failed_count, ai_used,
		       scores, checks, executive_summary, action_plan,
		       page_snapshot, sitewide, page_rows, listings, advanced,
		       generated_at
		FROM seo_audits WHERE id = $1
	`
	row := s.pool.QueryRow(ctx, query, id)

	var (
		resp       model.SEOAuditResponse
		scoresJSON, checksJSON, execJSON, actionJSON []byte
		snapshotJSON, sitewideJSON, rowsJSON []byte
		listingsJSON, advancedJSON []byte
		aiUsed bool
		passed, warnings, failed int
	)

	if err := row.Scan(
		&resp.ID, &resp.Domain, &resp.WebsiteURL, &resp.OverallScore, &resp.OverallGrade,
		&passed, &warnings, &failed, &aiUsed,
		&scoresJSON, &checksJSON, &execJSON, &actionJSON,
		&snapshotJSON, &sitewideJSON, &rowsJSON, &listingsJSON, &advancedJSON,
		&resp.GeneratedAt,
	); err != nil {
		return nil, fmt.Errorf("SEO audit %s not found: %w", id, err)
	}

	resp.MetricsSummary = model.MetricsSummary{Passed: passed, Warnings: warnings, Failed: failed}
	resp.AI = model.AIStatus{Available: aiUsed}
	resp.OverallGrade = resp.OverallGrade

	_ = json.Unmarshal(scoresJSON, &resp.Breakdown)
	_ = json.Unmarshal(checksJSON, &resp.Checks)
	_ = json.Unmarshal(execJSON, &resp.ExecutiveSummary)
	_ = json.Unmarshal(actionJSON, &resp.ActionPlan)
	_ = json.Unmarshal(snapshotJSON, &resp.PageSnapshot)
	_ = json.Unmarshal(sitewideJSON, &resp.Sitewide)
	_ = json.Unmarshal(rowsJSON, &resp.PageRows)
	_ = json.Unmarshal(listingsJSON, &resp.Listings)
	_ = json.Unmarshal(advancedJSON, &resp.Advanced)

	return &resp, nil
}

// GetSEOAuditsByDomain returns a list of audit summaries for a given domain.
func (s *StorageService) GetSEOAuditsByDomain(ctx context.Context, domain string) ([]model.SEOAuditSummary, error) {
	query := `
		SELECT id, overall_score, overall_grade, generated_at
		FROM seo_audits WHERE domain = $1
		ORDER BY generated_at DESC LIMIT 50
	`
	rows, err := s.pool.Query(ctx, query, domain)
	if err != nil {
		return nil, fmt.Errorf("query domain audits: %w", err)
	}
	defer rows.Close()

	var results []model.SEOAuditSummary
	for rows.Next() {
		var s model.SEOAuditSummary
		if err := rows.Scan(&s.ID, &s.OverallScore, &s.OverallGrade, &s.GeneratedAt); err != nil {
			continue
		}
		results = append(results, s)
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Authentication Storage
// ─────────────────────────────────────────────────────────────────────────────

func (s *StorageService) CreateUser(ctx context.Context, email, passwordHash string) (*model.User, error) {
	query := `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, created_at
	`
	var user model.User
	err := s.pool.QueryRow(ctx, query, email, passwordHash).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create user: %w", err)
	}
	return &user, nil
}

func (s *StorageService) CreateGoogleUser(ctx context.Context, email, googleID string) (*model.User, error) {
	query := `
		INSERT INTO users (email, google_id)
		VALUES ($1, $2)
		RETURNING id, email, google_id, created_at
	`
	var user model.User
	err := s.pool.QueryRow(ctx, query, email, googleID).Scan(
		&user.ID,
		&user.Email,
		&user.GoogleID,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create google user: %w", err)
	}
	return &user, nil
}

func (s *StorageService) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, email, password_hash, google_id, created_at
		FROM users
		WHERE email = $1
	`
	var user model.User
	err := s.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GoogleID,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err // Let caller check for pgx.ErrNoRows
	}
	return &user, nil
}
