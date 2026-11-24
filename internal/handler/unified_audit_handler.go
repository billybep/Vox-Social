package handler

import (
	"encoding/json"
	"net/http"
	"strings"

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

	userID := ""
	if uid, ok := r.Context().Value(UserIDKey).(string); ok {
		userID = uid
	}

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

// HandleAuditsRouter handles /api/v1/audits/ sub-paths:
// - POST /api/v1/audits/run
// - GET  /api/v1/audits/:id/status
// - POST /api/v1/audits/:id/retry-source
// - GET  /api/v1/audits/:id/results
// - GET  /api/v1/audits
func (h *UnifiedAuditHandler) HandleAuditsRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/audits")
	path = strings.Trim(path, "/")

	// GET /api/v1/audits
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

	// Split remaining path: e.g. ["audit_run_123", "status"]
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid audit route", http.StatusBadRequest)
		return
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
