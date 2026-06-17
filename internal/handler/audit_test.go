package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

// Mock Storage
type mockStorage struct {
	savedURL string
}

func (m *mockStorage) SaveAuditAsync(targetURL string, result *model.AnalyzeResponse) {
	m.savedURL = targetURL
}

// Mock AI Service
type mockAI struct {
	shouldFail bool
}

func (m *mockAI) AnalyzeProfile(ctx context.Context, profileURL string) (*model.AnalyzeResponse, error) {
	if m.shouldFail {
		return nil, context.DeadlineExceeded // Simulate a timeout or error
	}
	return &model.AnalyzeResponse{
		OverallScore:       99,
		Status:             "TEST",
		RecommendedPackage: "Vox Value",
	}, nil
}

func TestHandleAnalyze_Success(t *testing.T) {
	mockAI := &mockAI{shouldFail: false}
	mockStorage := &mockStorage{}
	handler := NewAuditHandler(mockAI, mockStorage)

	reqBody := `{"profile_url": "https://linkedin.com/in/test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleAnalyze(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK (200), got %d", res.StatusCode)
	}

	var resp model.AnalyzeResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.OverallScore != 99 {
		t.Errorf("Expected OverallScore 99, got %d", resp.OverallScore)
	}
	
	// Since SaveAuditAsync is called asynchronously, we shouldn't assert on it strictly in a fast unit test
	// without a wait group, but in standard execution it might have already triggered.
}

func TestHandleAnalyze_InvalidJSON(t *testing.T) {
	handler := NewAuditHandler(&mockAI{}, &mockStorage{})

	// Missing closing brace
	reqBody := `{"profile_url": "https://linkedin.com/in/test"`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handler.HandleAnalyze(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request (400), got %d", res.StatusCode)
	}
}

func TestHandleAnalyze_EmptyURL(t *testing.T) {
	handler := NewAuditHandler(&mockAI{}, &mockStorage{})

	reqBody := `{"profile_url": "   "}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handler.HandleAnalyze(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request (400), got %d", res.StatusCode)
	}
}

func TestHandleAnalyze_AIFailure(t *testing.T) {
	mockAI := &mockAI{shouldFail: true}
	handler := NewAuditHandler(mockAI, &mockStorage{})

	reqBody := `{"profile_url": "https://linkedin.com/in/test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()

	handler.HandleAnalyze(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status Internal Server Error (500), got %d", res.StatusCode)
	}
}
