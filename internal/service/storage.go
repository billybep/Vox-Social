package service

import (
	"context"
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
