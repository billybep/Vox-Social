package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/voxlumedia/vox-social-backend/internal/model"
	"github.com/voxlumedia/vox-social-backend/internal/service"
)

func TestHandleAuthSync_JSONPayload(t *testing.T) {
	authHandler := NewAuthHandler(nil) // nil storage uses graceful in-memory fallback

	payload := `{
		"uid": "fb_uid_test_99",
		"email": "dr.cedar@cedarandco.com",
		"displayName": "Dr. Cedar",
		"role": "agency_owner"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sync", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandler.HandleAuthSync(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp model.AuthSyncResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "synced" {
		t.Errorf("expected status 'synced', got '%s'", resp.Status)
	}
	if resp.User.UID != "fb_uid_test_99" {
		t.Errorf("expected user UID 'fb_uid_test_99', got '%s'", resp.User.UID)
	}
	if resp.User.Role != "agency_owner" {
		t.Errorf("expected role 'agency_owner', got '%s'", resp.User.Role)
	}
	if resp.Workspace.Credits != 50 {
		t.Errorf("expected workspace credits 50, got %d", resp.Workspace.Credits)
	}
}

func TestHandleAnalyze_URLFieldSupport(t *testing.T) {
	mockAI := &mockAI{shouldFail: false}
	mockStorage := &mockStorage{}
	auditHandler := NewAuditHandler(mockAI, mockStorage)

	// Test with "url" key instead of legacy "profile_url"
	payload := `{"url": "https://www.instagram.com/cedarandcodental"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	auditHandler.HandleAnalyze(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}
	if mockStorage.savedURL != "https://www.instagram.com/cedarandcodental" {
		t.Errorf("expected storage URL to be saved as https://www.instagram.com/cedarandcodental, got %s", mockStorage.savedURL)
	}
}

func TestUnifiedAuditWorkflow(t *testing.T) {
	unifiedSvc := service.NewUnifiedAuditService(nil, nil, nil)
	unifiedHandler := NewUnifiedAuditHandler(unifiedSvc)

	// 1. POST /api/v1/audits/run
	runPayload := `{
		"targetDomain": "cedarandcodental.com",
		"brandName": "Cedar & Co. Dental",
		"pillars": {
			"seo": true,
			"social": true,
			"ai": true,
			"cro": false
		},
		"languages": {
			"en": true,
			"ar": true
		},
		"competitors": [
			{ "name": "Tend Dental Studios", "domain": "hellotend.com", "active": true },
			{ "name": "Aspen Dental Care", "domain": "aspendental.com", "active": true }
		],
		"creditsDeducted": 12
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audits/run", bytes.NewBufferString(runPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	unifiedHandler.HandleRunAudit(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for run audit, got %d: %s", w.Code, w.Body.String())
	}

	var runResp model.AuditRunResponse
	if err := json.NewDecoder(w.Body).Decode(&runResp); err != nil {
		t.Fatalf("failed to decode run response: %v", err)
	}

	if runResp.AuditID == "" {
		t.Fatalf("expected non-empty auditId")
	}
	if runResp.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", runResp.Status)
	}
	if runResp.CreditsRemaining != 38 {
		t.Errorf("expected 38 credits remaining (50 - 12), got %d", runResp.CreditsRemaining)
	}

	auditID := runResp.AuditID

	// 2. GET /api/v1/audits/:id/status
	statusURL := "/api/v1/audits/" + auditID + "/status"
	reqStatus := httptest.NewRequest(http.MethodGet, statusURL, nil)
	wStatus := httptest.NewRecorder()

	unifiedHandler.HandleAuditsRouter(wStatus, reqStatus)

	if wStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for audit status, got %d: %s", wStatus.Code, wStatus.Body.String())
	}

	var statusResp model.AuditStatusResponse
	if err := json.NewDecoder(wStatus.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}

	if len(statusResp.Stages) != 5 {
		t.Errorf("expected 5 stages, got %d", len(statusResp.Stages))
	}
	if statusResp.AuditID != auditID {
		t.Errorf("expected auditId '%s', got '%s'", auditID, statusResp.AuditID)
	}

	// 3. POST /api/v1/audits/:id/retry-source
	retryURL := "/api/v1/audits/" + auditID + "/retry-source"
	retryPayload := `{"stageId": "stage-2"}`
	reqRetry := httptest.NewRequest(http.MethodPost, retryURL, bytes.NewBufferString(retryPayload))
	reqRetry.Header.Set("Content-Type", "application/json")
	wRetry := httptest.NewRecorder()

	unifiedHandler.HandleAuditsRouter(wRetry, reqRetry)

	if wRetry.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for retry source, got %d: %s", wRetry.Code, wRetry.Body.String())
	}

	// 4. GET /api/v1/audits/:id/results
	resultsURL := "/api/v1/audits/" + auditID + "/results"
	reqResults := httptest.NewRequest(http.MethodGet, resultsURL, nil)
	wResults := httptest.NewRecorder()

	unifiedHandler.HandleAuditsRouter(wResults, reqResults)

	if wResults.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for audit results, got %d: %s", wResults.Code, wResults.Body.String())
	}

	var resultsResp model.UnifiedAuditResult
	if err := json.NewDecoder(wResults.Body).Decode(&resultsResp); err != nil {
		t.Fatalf("failed to decode results response: %v", err)
	}

	if resultsResp.OverallScore <= 0 {
		t.Errorf("expected positive overall score, got %d", resultsResp.OverallScore)
	}
	if len(resultsResp.Competitors) < 2 {
		t.Errorf("expected at least 2 competitors, got %d", len(resultsResp.Competitors))
	}
	if len(resultsResp.KeyFindings) == 0 {
		t.Errorf("expected key findings, got none")
	}

	// 5. GET /api/v1/audits (list audits)
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/audits", nil)
	wList := httptest.NewRecorder()

	unifiedHandler.HandleAuditsRouter(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for audits list, got %d: %s", wList.Code, wList.Body.String())
	}
}
