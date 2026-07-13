package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

// SEOAuditService is the interface the handler depends on.
type SEOAuditService interface {
	RunAudit(ctx context.Context, req model.SEOAuditRequest) (*model.SEOAuditResponse, error)
	GetAuditByID(ctx context.Context, id string) (*model.SEOAuditResponse, error)
	GetAuditsByDomain(ctx context.Context, domain string) ([]model.SEOAuditSummary, error)
}

// SEOAuditHandler handles all SEO audit HTTP routes.
type SEOAuditHandler struct {
	svc SEOAuditService
}

// NewSEOAuditHandler creates a new SEOAuditHandler.
func NewSEOAuditHandler(svc SEOAuditService) *SEOAuditHandler {
	return &SEOAuditHandler{svc: svc}
}

// HandleAudit serves POST /api/v1/seo/audit
func (h *SEOAuditHandler) HandleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.SEOAuditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Normalize domain: strip scheme, path, trailing slash
	req.Domain = strings.TrimSpace(req.Domain)
	req.Domain = strings.TrimPrefix(req.Domain, "https://")
	req.Domain = strings.TrimPrefix(req.Domain, "http://")
	req.Domain = strings.TrimPrefix(req.Domain, "www.")
	req.Domain = strings.TrimRight(req.Domain, "/")

	if req.Domain == "" {
		jsonError(w, "domain is required", http.StatusBadRequest)
		return
	}

	log.Printf("[SEO HANDLER] Audit request for domain: %s", req.Domain)

	// Long-running: allow up to 150 seconds for the full pipeline
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	resp, err := h.svc.RunAudit(ctx, req)
	if err != nil {
		log.Printf("[SEO HANDLER] Error: %v", err)
		jsonError(w, "Audit failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, resp)
}

// HandleGetAudit serves GET /api/v1/seo/audit/{id}
func (h *SEOAuditHandler) HandleGetAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract {id} from path: /api/v1/seo/audit/
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/seo/audit/")
	id := strings.Trim(path, "/")
	if id == "" {
		jsonError(w, "audit id is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := h.svc.GetAuditByID(ctx, id)
	if err != nil {
		jsonError(w, "Audit not found: "+err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, resp)
}

// HandleGetByDomain serves GET /api/v1/seo/domain/{domain}
func (h *SEOAuditHandler) HandleGetByDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/seo/domain/")
	domain := strings.Trim(path, "/")
	if domain == "" {
		jsonError(w, "domain is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	audits, err := h.svc.GetAuditsByDomain(ctx, domain)
	if err != nil {
		jsonError(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type domainHistoryResponse struct {
		Domain string                   `json:"domain"`
		Audits []model.SEOAuditSummary  `json:"audits"`
	}
	jsonOK(w, domainHistoryResponse{Domain: domain, Audits: audits})
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
