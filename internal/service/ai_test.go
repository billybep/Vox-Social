package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnalyzeProfile_Success(t *testing.T) {
	// Mock Scraper Server
	scraperServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"bio": "Test bio", "followers": 1000}`))
	}))
	defer scraperServer.Close()

	// Mock AI Server
	aiResponse := `
	{
		"choices": [{
			"message": {
				"content": "{\"overall_score\": 85, \"status\": \"EXCELLENT\", \"profile_identity\": \"Tech Leader\", \"growth_potential\": \"High\", \"profile_readiness\": 95, \"key_strengths\": [\"Leadership\"], \"opportunities\": [\"Speaking\"], \"recommended_package\": \"Vox Premier\"}"
			}
		}]
	}`

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(aiResponse))
	}))
	defer aiServer.Close()

	aiService := NewAIService("ai-key", aiServer.URL, "test-model", "scraper-key", scraperServer.URL, scraperServer.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := aiService.AnalyzeProfile(ctx, "https://twitter.com/test_user")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.RecommendedPackage != "Vox Premier" {
		t.Errorf("Expected Vox Premier, got %s", result.RecommendedPackage)
	}
}
