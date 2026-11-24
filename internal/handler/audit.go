package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type AI interface {
	AnalyzeProfile(ctx context.Context, profileURL string) (*model.AnalyzeResponse, error)
}

type Storage interface {
	SaveAuditAsync(targetURL string, result *model.AnalyzeResponse)
	SaveAuditRecordAsync(record *model.AuditRecord)
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

	// Check for empty strings (support both profile_url and url)
	targetURL := strings.TrimSpace(req.ProfileURL)
	if targetURL == "" {
		targetURL = strings.TrimSpace(req.URL)
	}
	if targetURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "url or profile_url is required"})
		return
	}
	req.ProfileURL = targetURL

	// Use context.Background() so that client disconnection (e.g. Insomnia timeout)
	// does NOT cancel the AI work. The 120s timeout is our own safety net.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	aiResult, err := h.aiService.AnalyzeProfile(ctx, req.ProfileURL)
	if err != nil {
		log.Printf("ERROR analyzing profile: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to analyze profile: " + err.Error()})
		return
	}

	// 3. Spin up a Goroutine to save this record into PostgreSQL (using background context in storageService)
	h.storageService.SaveAuditAsync(req.ProfileURL, aiResult)

	// Auto-save to persistent audit_records table
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = ExtractUserID(r)
	}

	socialScore := aiResult.OverallScore
	overallScore := aiResult.OverallScore
	targetDomain := extractDomainFromURL(req.ProfileURL)
	brandName := strings.TrimSpace(aiResult.ProfileIdentity)
	if brandName == "" {
		brandName = targetDomain
	}
	fullDataBytes, _ := json.Marshal(aiResult)

	auditRecord := &model.AuditRecord{
		ID:               uuid.New().String(),
		UserID:           userID,
		AuditType:        "social",
		TargetDomain:     targetDomain,
		BrandName:        brandName,
		SocialAccount:    req.ProfileURL,
		OverallScore:     overallScore,
		SEOScore:         nil,
		SocialScore:      &socialScore,
		Status:           "completed",
		EvidenceCoverage: 92,
		FullData:         fullDataBytes,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	h.storageService.SaveAuditRecordAsync(auditRecord)

	// 4. Concurrently and immediately return the 200 OK JSON response back to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(aiResult)
}

func extractDomainFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "www.")
	parts := strings.Split(rawURL, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "social.profile"
}
