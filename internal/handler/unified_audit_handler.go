package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/voxlumedia/vox-social-backend/internal/model"
	"github.com/voxlumedia/vox-social-backend/internal/service"
)

type UnifiedAuditHandler struct {
	svc *service.UnifiedAuditService
}

func NewUnifiedAuditHandler(svc *service.UnifiedAuditService) *UnifiedAuditHandler {
	return &UnifiedAuditHandler{svc: svc}
}

// HandleRunAudit handles POST /api/v1/audits/run
func (h *UnifiedAuditHandler) HandleRunAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.AuditRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON payload: " + err.Error()})
		return
	}

	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = ExtractUserID(r)
	}
	req.UserID = userID

	resp, err := h.svc.CreateAudit(r.Context(), req, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleGetHistory handles GET /api/v1/audits/history
func (h *UnifiedAuditHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := ExtractUserID(r)
	auditType := strings.TrimSpace(r.URL.Query().Get("type"))
	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	storage := h.svc.GetStorage()
	var history []model.AuditRecordSummary
	var err error
	if storage != nil {
		history, err = storage.GetAuditHistory(r.Context(), userID, auditType, limit)
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if history == nil {
		history = []model.AuditRecordSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model.AuditHistoryResponse{
		Success: true,
		Data:    history,
		Total:   len(history),
	})
}

// HandleGetLatest handles GET /api/v1/audits/latest
func (h *UnifiedAuditHandler) HandleGetLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := ExtractUserID(r)
	storage := h.svc.GetStorage()
	var latest *model.AuditRecord
	var err error
	if storage != nil {
		latest, err = storage.GetLatestAudit(r.Context(), userID)
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model.AuditLatestResponse{
		Success: true,
		Data:    latest,
	})
}

// HandleGetAuditByID handles GET /api/v1/audits/:id
func (h *UnifiedAuditHandler) HandleGetAuditByID(w http.ResponseWriter, r *http.Request, auditID string) {
	storage := h.svc.GetStorage()
	var record *model.AuditRecord
	var err error

	if storage != nil {
		record, err = storage.GetAuditRecordByID(r.Context(), auditID)
	}

	// If not found in storage, check active in-memory unified audits
	if record == nil {
		if result, errUnified := h.svc.GetResults(auditID); errUnified == nil && result != nil {
			fullDataBytes, _ := json.Marshal(result)
			seoScore := result.SEOScore
			socialScore := result.SocialScore
			record = &model.AuditRecord{
				ID:               auditID,
				UserID:           ExtractUserID(r),
				AuditType:        "unified",
				TargetDomain:     result.TargetDomain,
				BrandName:        result.BrandName,
				OverallScore:     result.OverallScore,
				SEOScore:         &seoScore,
				SocialScore:      &socialScore,
				Status:           result.OverallStatus,
				EvidenceCoverage: 92,
				FullData:         fullDataBytes,
				CreatedAt:        result.StartedAt,
				UpdatedAt:        time.Now(),
			}
			err = nil
		}
	}

	if record == nil || err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(model.AuditDetailResponse{
			Success: false,
			Error:   "Audit record not found: " + auditID,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model.AuditDetailResponse{
		Success: true,
		Data:    record,
	})
}

// HandleAuditsRouter handles /api/v1/audits/ sub-paths:
// - POST /api/v1/audits/run
// - GET  /api/v1/audits/history
// - GET  /api/v1/audits/latest
// - GET  /api/v1/audits/:id
// - GET  /api/v1/audits/:id/status
// - POST /api/v1/audits/:id/retry-source
// - GET  /api/v1/audits/:id/results
// - GET  /api/v1/audits
func (h *UnifiedAuditHandler) HandleAuditsRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/audits")
	path = strings.Trim(path, "/")

	// GET /api/v1/audits (all audits list)
	if path == "" {
		if r.Method == http.MethodGet {
			audits := h.svc.ListAudits()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"audits": audits,
				"count":  len(audits),
			})
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// POST /api/v1/audits/run
	if path == "run" {
		h.HandleRunAudit(w, r)
		return
	}

	parts := strings.Split(path, "/")

	// Single part routes: history, latest, or :id
	if len(parts) == 1 {
		switch parts[0] {
		case "history":
			h.HandleGetHistory(w, r)
			return
		case "latest":
			h.HandleGetLatest(w, r)
			return
		default:
			if r.Method == http.MethodGet {
				h.HandleGetAuditByID(w, r, parts[0])
				return
			}
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}

	auditID := parts[0]
	action := parts[1]

	switch action {
	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status, err := h.svc.GetStatus(auditID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(status)

	case "retry-source":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var retryReq model.RetrySourceRequest
		if err := json.NewDecoder(r.Body).Decode(&retryReq); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON payload"})
			return
		}
		updatedStatus, err := h.svc.RetrySource(auditID, retryReq.StageID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(updatedStatus)

	case "results":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		results, err := h.svc.GetResults(auditID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)

	default:
		http.Error(w, "Unknown audit action: "+action, http.StatusNotFound)
	}
}
