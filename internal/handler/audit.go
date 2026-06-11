package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type AI interface {
	AnalyzeProfile(ctx context.Context, profileURL string) (*model.AnalyzeResponse, error)
}

type Storage interface {
	SaveAuditAsync(targetURL string, result *model.AnalyzeResponse)
}

type AuditHandler struct {
	aiService      AI
	storageService Storage
}

func NewAuditHandler(ai AI, storage Storage) *AuditHandler {
	return &AuditHandler{
		aiService:      ai,
		storageService: storage,
	}
}

func (h *AuditHandler) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req model.AnalyzeRequest
	// 1. Validate incoming JSON
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON payload"})
		return
	}

	// Check for empty strings
	req.ProfileURL = strings.TrimSpace(req.ProfileURL)
	if req.ProfileURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "profile_url is required"})
		return
	}

	// 2. Mock/execute HTTP POST to AI API with 10-second context timeout (done in Service layer)
	aiResult, err := h.aiService.AnalyzeProfile(r.Context(), req.ProfileURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to analyze profile"})
		return
	}

	// 3. Spin up a Goroutine to save this record into PostgreSQL (using background context in storageService)
	h.storageService.SaveAuditAsync(req.ProfileURL, aiResult)

	// 4. Concurrently and immediately return the 200 OK JSON response back to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(aiResult)
}
