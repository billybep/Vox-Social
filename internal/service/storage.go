package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type StorageService struct {
	pool            *pgxpool.Pool
	mu              sync.RWMutex
	inMemoryRecords []*model.AuditRecord
	recordMap       map[string]*model.AuditRecord
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

	svc := &StorageService{
		pool:      pool,
		recordMap: make(map[string]*model.AuditRecord),
	}

	// Auto-migrate audit_records table
	if err := svc.AutoMigrate(ctx); err != nil {
		log.Printf("[DB WARNING] AutoMigrate audit_records failed: %v", err)
	}

	return svc, nil
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

// ─────────────────────────────────────────────────────────────────────────────
// Unified Audit Records Storage (Persistent History)
// ─────────────────────────────────────────────────────────────────────────────

// AutoMigrate creates the audit_records table and required indexes if they do not exist.
func (s *StorageService) AutoMigrate(ctx context.Context) error {
	if s.pool == nil {
		return nil
	}
	query := `
		CREATE TABLE IF NOT EXISTS audit_records (
			id VARCHAR(255) PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			audit_type VARCHAR(50) NOT NULL,
			target_domain TEXT NOT NULL,
			brand_name TEXT NOT NULL,
			social_account TEXT DEFAULT '',
			overall_score INT NOT NULL DEFAULT 0,
			seo_score INT,
			social_score INT,
			status VARCHAR(50) NOT NULL DEFAULT 'completed',
			evidence_coverage INT NOT NULL DEFAULT 92,
			full_data JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_audit_records_user_id ON audit_records(user_id);
		CREATE INDEX IF NOT EXISTS idx_audit_records_created_at ON audit_records(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_records_type ON audit_records(audit_type);
	`
	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("auto-migrate audit_records: %w", err)
	}
	log.Println("✅ [DB] audit_records table and indexes auto-migrated successfully")
	return nil
}

// SaveAuditRecord persists or updates an audit record in PostgreSQL and in-memory cache.
func (s *StorageService) SaveAuditRecord(ctx context.Context, record *model.AuditRecord) error {
	if record == nil {
		return fmt.Errorf("record cannot be nil")
	}

	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	record.UpdatedAt = time.Now()
	if record.EvidenceCoverage == 0 {
		record.EvidenceCoverage = 92
	}
	if record.Status == "" {
		record.Status = "completed"
	}

	// In-memory cache update
	s.mu.Lock()
	if s.recordMap == nil {
		s.recordMap = make(map[string]*model.AuditRecord)
	}
	existing, found := s.recordMap[record.ID]
	if found {
		*existing = *record
	} else {
		recCopy := *record
		s.recordMap[record.ID] = &recCopy
		s.inMemoryRecords = append([]*model.AuditRecord{&recCopy}, s.inMemoryRecords...)
	}
	s.mu.Unlock()

	if s.pool == nil {
		return nil
	}

	query := `
		INSERT INTO audit_records (
			id, user_id, audit_type, target_domain, brand_name, social_account,
			overall_score, seo_score, social_score, status, evidence_coverage,
			full_data, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			audit_type = EXCLUDED.audit_type,
			target_domain = EXCLUDED.target_domain,
			brand_name = EXCLUDED.brand_name,
			social_account = EXCLUDED.social_account,
			overall_score = EXCLUDED.overall_score,
			seo_score = EXCLUDED.seo_score,
			social_score = EXCLUDED.social_score,
			status = EXCLUDED.status,
			evidence_coverage = EXCLUDED.evidence_coverage,
			full_data = EXCLUDED.full_data,
			updated_at = EXCLUDED.updated_at
	`
	_, err := s.pool.Exec(ctx, query,
		record.ID,
		record.UserID,
		record.AuditType,
		record.TargetDomain,
		record.BrandName,
		record.SocialAccount,
		record.OverallScore,
		record.SEOScore,
		record.SocialScore,
		record.Status,
		record.EvidenceCoverage,
		record.FullData,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		log.Printf("[DB ERROR] Failed to save audit_record %s: %v", record.ID, err)
		return err
	}
	log.Printf("[DB] Successfully saved audit_record: %s (%s - %s)", record.ID, record.AuditType, record.TargetDomain)
	return nil
}

// SaveAuditRecordAsync runs SaveAuditRecord in a standalone background goroutine.
func (s *StorageService) SaveAuditRecordAsync(record *model.AuditRecord) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.SaveAuditRecord(ctx, record)
	}()
}

// GetAuditHistory loads the audit history for a user, sorted by created_at DESC.
func (s *StorageService) GetAuditHistory(ctx context.Context, userID, auditType string, limit int) ([]model.AuditRecordSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var results []model.AuditRecordSummary

	if s.pool != nil {
		var query string
		var rows pgx.Rows
		var err error

		if auditType != "" {
			query = `
				SELECT id, user_id, audit_type, target_domain, brand_name, social_account,
				       overall_score, seo_score, social_score, status, evidence_coverage, created_at
				FROM audit_records
				WHERE user_id = $1 AND audit_type = $2
				ORDER BY created_at DESC
				LIMIT $3
			`
			rows, err = s.pool.Query(ctx, query, userID, auditType, limit)
		} else {
			query = `
				SELECT id, user_id, audit_type, target_domain, brand_name, social_account,
				       overall_score, seo_score, social_score, status, evidence_coverage, created_at
				FROM audit_records
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT $2
			`
			rows, err = s.pool.Query(ctx, query, userID, limit)
		}

		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var rec model.AuditRecordSummary
				if scanErr := rows.Scan(
					&rec.ID, &rec.UserID, &rec.AuditType, &rec.TargetDomain, &rec.BrandName, &rec.SocialAccount,
					&rec.OverallScore, &rec.SEOScore, &rec.SocialScore, &rec.Status, &rec.EvidenceCoverage, &rec.CreatedAt,
				); scanErr == nil {
					results = append(results, rec)
				}
			}
			return results, nil
		}
		log.Printf("[DB WARNING] GetAuditHistory query failed: %v (falling back to in-memory)", err)
	}

	// Fallback to in-memory store
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rec := range s.inMemoryRecords {
		if (userID == "" || rec.UserID == userID || rec.UserID == "anonymous" || userID == "anonymous") &&
			(auditType == "" || rec.AuditType == auditType) {
			results = append(results, model.AuditRecordSummary{
				ID:               rec.ID,
				UserID:           rec.UserID,
				AuditType:        rec.AuditType,
				TargetDomain:     rec.TargetDomain,
				BrandName:        rec.BrandName,
				SocialAccount:    rec.SocialAccount,
				OverallScore:     rec.OverallScore,
				SEOScore:         rec.SEOScore,
				SocialScore:      rec.SocialScore,
				Status:           rec.Status,
				EvidenceCoverage: rec.EvidenceCoverage,
				CreatedAt:        rec.CreatedAt,
			})
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// GetLatestAudit returns the single most recent completed audit for a given user.
func (s *StorageService) GetLatestAudit(ctx context.Context, userID string) (*model.AuditRecord, error) {
	if s.pool != nil {
		query := `
			SELECT id, user_id, audit_type, target_domain, brand_name, social_account,
			       overall_score, seo_score, social_score, status, evidence_coverage,
			       full_data, created_at, updated_at
			FROM audit_records
			WHERE user_id = $1 AND status = 'completed'
			ORDER BY created_at DESC
			LIMIT 1
		`
		row := s.pool.QueryRow(ctx, query, userID)
		var rec model.AuditRecord
		err := row.Scan(
			&rec.ID, &rec.UserID, &rec.AuditType, &rec.TargetDomain, &rec.BrandName, &rec.SocialAccount,
			&rec.OverallScore, &rec.SEOScore, &rec.SocialScore, &rec.Status, &rec.EvidenceCoverage,
			&rec.FullData, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err == nil {
			return &rec, nil
		}
		if err != pgx.ErrNoRows {
			log.Printf("[DB WARNING] GetLatestAudit failed: %v (falling back to in-memory)", err)
		}
	}

	// Fallback to in-memory store
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, rec := range s.inMemoryRecords {
		if (userID == "" || rec.UserID == userID || rec.UserID == "anonymous" || userID == "anonymous") && rec.Status == "completed" {
			recCopy := *rec
			return &recCopy, nil
		}
	}

	return nil, nil
}

// GetAuditRecordByID loads a specific audit record by its unique ID.
func (s *StorageService) GetAuditRecordByID(ctx context.Context, id string) (*model.AuditRecord, error) {
	if s.pool != nil {
		query := `
			SELECT id, user_id, audit_type, target_domain, brand_name, social_account,
			       overall_score, seo_score, social_score, status, evidence_coverage,
			       full_data, created_at, updated_at
			FROM audit_records
			WHERE id = $1
		`
		row := s.pool.QueryRow(ctx, query, id)
		var rec model.AuditRecord
		err := row.Scan(
			&rec.ID, &rec.UserID, &rec.AuditType, &rec.TargetDomain, &rec.BrandName, &rec.SocialAccount,
			&rec.OverallScore, &rec.SEOScore, &rec.SocialScore, &rec.Status, &rec.EvidenceCoverage,
			&rec.FullData, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err == nil {
			return &rec, nil
		}
		if err != pgx.ErrNoRows {
			log.Printf("[DB WARNING] GetAuditRecordByID failed: %v", err)
		}
	}

	// Fallback to in-memory store
	s.mu.RLock()
	defer s.mu.RUnlock()

	if rec, found := s.recordMap[id]; found {
		recCopy := *rec
		return &recCopy, nil
	}

	return nil, fmt.Errorf("audit record %s not found", id)
}
