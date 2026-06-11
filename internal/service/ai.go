package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type AIService struct {
	APIKey   string
	Endpoint string
	Client   *http.Client
}

func NewAIService(apiKey, endpoint string) *AIService {
	return &AIService{
		APIKey:   apiKey,
		Endpoint: endpoint,
		Client: &http.Client{
			Timeout: 10 * time.Second, // Strict 10-second timeout
		},
	}
}

// AnalyzeProfile calls an external AI/Scraper to analyze the given URL
func (s *AIService) AnalyzeProfile(ctx context.Context, profileURL string) (*model.AnalyzeResponse, error) {
	// For production, if the external API is not available, we mock the behavior
	if s.Endpoint == "https://api.example.com/v1/analyze" {
		// Mock behavior to simulate network delay
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		
		return &model.AnalyzeResponse{
			OverallScore:     73,
			Status:           "GOOD",
			ProfileIdentity:  "A dynamic and growing tech profile.",
			GrowthPotential:  "Moderate",
			ProfileReadiness: 87,
			KeyStrengths:     []string{"Consistent posting", "High engagement rate"},
			Opportunities:    []string{"Add more video content", "Engage with niche influencers"},
		}, nil
	}

	payload := map[string]string{"url": profileURL}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.APIKey)
	}

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external AI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code from AI: %d", resp.StatusCode)
	}

	var result model.AnalyzeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode AI response: %w", err)
	}

	return &result, nil
}
