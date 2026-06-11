package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAnalyzeProfile_Success(t *testing.T) {
	// Create a mock server that returns a valid JSON response
	mockResponse := `
	{
		"overall_score": 85,
		"status": "EXCELLENT",
		"profile_identity": "Tech Leader",
		"growth_potential": "High",
		"profile_readiness": 95,
		"key_strengths": ["Leadership", "Coding"],
		"opportunities": ["Public Speaking"]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and headers
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("Expected valid Authorization header")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	// Initialize our AI Service pointing to the test server
	aiService := NewAIService("test-api-key", server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := aiService.AnalyzeProfile(ctx, "https://twitter.com/test_user")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.OverallScore != 85 {
		t.Errorf("Expected OverallScore 85, got %d", result.OverallScore)
	}
	if result.Status != "EXCELLENT" {
		t.Errorf("Expected Status EXCELLENT, got %s", result.Status)
	}
}

func TestAnalyzeProfile_MockBehavior(t *testing.T) {
	// If endpoint is our mocked endpoint, it should return default mock data
	aiService := NewAIService("", "https://api.example.com/v1/analyze")

	ctx := context.Background()
	result, err := aiService.AnalyzeProfile(ctx, "https://example.com/profile")

	if err != nil {
		t.Fatalf("Expected no error from mock behavior, got %v", err)
	}

	if result.OverallScore != 73 {
		t.Errorf("Expected OverallScore 73, got %d", result.OverallScore)
	}
}
