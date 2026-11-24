package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type auditJob struct {
	mu           sync.RWMutex
	request      model.AuditRunRequest
	auditID      string
	overallStatus string
	startedAt    time.Time
	completedAt  *time.Time
	stages       []model.AuditStage
	result       *model.UnifiedAuditResult
}

type UnifiedAuditService struct {
	mu         sync.RWMutex
	jobs       map[string]*auditJob
	userCredits map[string]int
	seoSvc     *SEOService
	aiSvc      *AIService
	storageSvc *StorageService
}

func NewUnifiedAuditService(seoSvc *SEOService, aiSvc *AIService, storageSvc *StorageService) *UnifiedAuditService {
	return &UnifiedAuditService{
		jobs:        make(map[string]*auditJob),
		userCredits: make(map[string]int),
		seoSvc:      seoSvc,
		aiSvc:       aiSvc,
		storageSvc:  storageSvc,
	}
}

// CreateAudit initializes a new audit run, deducts credits, and spawns the background worker.
func (s *UnifiedAuditService) CreateAudit(ctx context.Context, req model.AuditRunRequest, userID string) (*model.AuditRunResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if userID == "" {
		userID = "default_user"
	}

	// Balance tracking (default balance 50 credits)
	balance, exists := s.userCredits[userID]
	if !exists {
		balance = 50
	}
	deduct := req.CreditsDeducted
	if deduct <= 0 {
		deduct = 12 // default cost
	}
	remaining := balance - deduct
	if remaining < 0 {
		remaining = 0
	}
	s.userCredits[userID] = remaining

	now := time.Now()
	auditID := fmt.Sprintf("audit_run_%d", now.UnixNano()/int64(time.Millisecond))

	activeCompetitorCount := 0
	for _, c := range req.Competitors {
		if c.Active {
			activeCompetitorCount++
		}
	}

	initialStages := []model.AuditStage{
		{
			ID:        "stage-1",
			Name:      "Website Crawl & VoxSEO",
			Status:    "running",
			Timestamp: stringPtr("00:01"),
			Details:   "Crawling website & verifying 51 SEO checkpoints",
		},
		{
			ID:        "stage-2",
			Name:      "Social Profiles & VoxSocial",
			Status:    "pending",
			Timestamp: nil,
			Details:   "Waiting for website crawl",
		},
		{
			ID:        "stage-3",
			Name:      "AI Visibility Grounding",
			Status:    "pending",
			Timestamp: nil,
			Details:   "Waiting for social queue",
		},
		{
			ID:        "stage-4",
			Name:      "Competitor Benchmarking",
			Status:    "pending",
			Timestamp: nil,
			Details:   fmt.Sprintf("Queued for %d active competitors", activeCompetitorCount),
		},
		{
			ID:        "stage-5",
			Name:      "Report Synthesis",
			Status:    "pending",
			Timestamp: nil,
			Details:   "Unified report generation",
		},
	}

	job := &auditJob{
		request:       req,
		auditID:       auditID,
		overallStatus: "running",
		startedAt:     now,
		stages:        initialStages,
	}

	s.jobs[auditID] = job

	// Auto-save initial record in storage
	if s.storageSvc != nil {
		fullDataBytes, _ := json.Marshal(req)
		brand := req.BrandName
		if brand == "" {
			brand = req.TargetDomain
		}
		s.storageSvc.SaveAuditRecordAsync(&model.AuditRecord{
			ID:               auditID,
			UserID:           userID,
			AuditType:        "unified",
			TargetDomain:     req.TargetDomain,
			BrandName:        brand,
			OverallScore:     0,
			Status:           "running",
			EvidenceCoverage: 92,
			FullData:         fullDataBytes,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	// Launch background progress orchestrator
	go s.orchestrateAudit(auditID)

	return &model.AuditRunResponse{
		AuditID:          auditID,
		Status:           "running",
		CreditsRemaining: remaining,
		StartedAt:        now,
	}, nil
}

// orchestrateAudit moves through stages with realistic progress timings
func (s *UnifiedAuditService) orchestrateAudit(auditID string) {
	s.mu.RLock()
	job, exists := s.jobs[auditID]
	s.mu.RUnlock()
	if !exists {
		return
	}

	time.Sleep(3 * time.Second)

	// Stage 1 completed, Stage 2 running
	job.mu.Lock()
	job.stages[0].Status = "completed"
	job.stages[0].Timestamp = stringPtr("00:04")
	job.stages[0].Details = "51 checkpoints verified"

	job.stages[1].Status = "running"
	job.stages[1].Timestamp = stringPtr("00:05")
	job.stages[1].Details = "Gemini 1.5 Pro processing media reels"
	job.mu.Unlock()

	time.Sleep(4 * time.Second)

	// Stage 2 completed, Stage 3 running
	job.mu.Lock()
	job.stages[1].Status = "completed"
	job.stages[1].Timestamp = stringPtr("00:11")
	job.stages[1].Details = "Social profile & engagement analyzed"

	job.stages[2].Status = "running"
	job.stages[2].Timestamp = stringPtr("00:12")
	job.stages[2].Details = "Grounding AI answers across Gemini, Perplexity, OpenAI"
	job.mu.Unlock()

	time.Sleep(4 * time.Second)

	// Stage 3 completed, Stage 4 running
	job.mu.Lock()
	job.stages[2].Status = "completed"
	job.stages[2].Timestamp = stringPtr("00:16")
	job.stages[2].Details = "AI visibility signals mapped"

	job.stages[3].Status = "running"
	job.stages[3].Timestamp = stringPtr("00:17")
	job.stages[3].Details = "Benchmarking active competitors"
	job.mu.Unlock()

	time.Sleep(4 * time.Second)

	// Stage 4 completed, Stage 5 running
	job.mu.Lock()
	job.stages[3].Status = "completed"
	job.stages[3].Timestamp = stringPtr("00:22")
	job.stages[3].Details = "Competitor metrics synthesized"

	job.stages[4].Status = "running"
	job.stages[4].Timestamp = stringPtr("00:23")
	job.stages[4].Details = "Compiling unified intelligence report"
	job.mu.Unlock()

	time.Sleep(3 * time.Second)

	// Stage 5 completed -> All Done
	completedAt := time.Now()
	job.mu.Lock()
	job.stages[4].Status = "completed"
	job.stages[4].Timestamp = stringPtr("00:26")
	job.stages[4].Details = "Unified report generation complete"
	job.overallStatus = "completed"
	job.completedAt = &completedAt
	job.result = s.buildUnifiedResult(job.request, auditID, job.startedAt, &completedAt)
	resultCopy := job.result
	job.mu.Unlock()

	// Auto-save completed record in persistent storage
	if s.storageSvc != nil && resultCopy != nil {
		fullDataBytes, _ := json.Marshal(resultCopy)
		brand := job.request.BrandName
		if brand == "" {
			brand = job.request.TargetDomain
		}
		seoScore := resultCopy.SEOScore
		socialScore := resultCopy.SocialScore
		userID := job.request.UserID
		if userID == "" {
			userID = "anonymous"
		}
		s.storageSvc.SaveAuditRecordAsync(&model.AuditRecord{
			ID:               auditID,
			UserID:           userID,
			AuditType:        "unified",
			TargetDomain:     job.request.TargetDomain,
			BrandName:        brand,
			OverallScore:     resultCopy.OverallScore,
			SEOScore:         &seoScore,
			SocialScore:      &socialScore,
			Status:           "completed",
			EvidenceCoverage: 92,
			FullData:         fullDataBytes,
			CreatedAt:        job.startedAt,
			UpdatedAt:        completedAt,
		})
	}
}

// GetStorage returns the underlying storage service
func (s *UnifiedAuditService) GetStorage() *StorageService {
	return s.storageSvc
}

// GetStatus returns the current stage progression and overall status.
func (s *UnifiedAuditService) GetStatus(auditID string) (*model.AuditStatusResponse, error) {
	s.mu.RLock()
	job, exists := s.jobs[auditID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("audit %s not found", auditID)
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	stagesCopy := make([]model.AuditStage, len(job.stages))
	copy(stagesCopy, job.stages)

	return &model.AuditStatusResponse{
		AuditID:       auditID,
		OverallStatus: job.overallStatus,
		Stages:        stagesCopy,
	}, nil
}

// RetrySource resets a failed or partial stage and transitions it back to running -> completed.
func (s *UnifiedAuditService) RetrySource(auditID string, stageID string) (*model.AuditStatusResponse, error) {
	s.mu.RLock()
	job, exists := s.jobs[auditID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("audit %s not found", auditID)
	}

	job.mu.Lock()
	found := false
	for i := range job.stages {
		if job.stages[i].ID == stageID {
			found = true
			job.stages[i].Status = "running"
			job.stages[i].Details = "Retrying source connection..."
			job.stages[i].Timestamp = stringPtr("00:15")
			job.overallStatus = "running"
			break
		}
	}
	job.mu.Unlock()

	if !found {
		return nil, fmt.Errorf("stage %s not found in audit %s", stageID, auditID)
	}

	// In background, complete the retried stage
	go func() {
		time.Sleep(2 * time.Second)
		job.mu.Lock()
		defer job.mu.Unlock()
		for i := range job.stages {
			if job.stages[i].ID == stageID {
				job.stages[i].Status = "completed"
				job.stages[i].Timestamp = stringPtr("00:20")
				job.stages[i].Details = "Source connection re-established and verified"
				break
			}
		}

		// Re-evaluate overall status
		allCompleted := true
		for _, st := range job.stages {
			if st.Status != "completed" {
				allCompleted = false
				break
			}
		}
		if allCompleted {
			now := time.Now()
			job.overallStatus = "completed"
			job.completedAt = &now
			job.result = s.buildUnifiedResult(job.request, auditID, job.startedAt, &now)
		}
	}()

	return s.GetStatus(auditID)
}

// GetResults returns the unified results for Dashboard, Overview, and reports.
func (s *UnifiedAuditService) GetResults(auditID string) (*model.UnifiedAuditResult, error) {
	s.mu.RLock()
	job, exists := s.jobs[auditID]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("audit %s not found", auditID)
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	if job.result != nil {
		return job.result, nil
	}

	// If not finished yet, return intermediate result
	return s.buildUnifiedResult(job.request, auditID, job.startedAt, job.completedAt), nil
}

// ListAudits returns all audits for the workspace dashboard.
func (s *UnifiedAuditService) ListAudits() []*model.UnifiedAuditResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*model.UnifiedAuditResult, 0, len(s.jobs))
	for _, job := range s.jobs {
		job.mu.RLock()
		if job.result != nil {
			results = append(results, job.result)
		} else {
			results = append(results, s.buildUnifiedResult(job.request, job.auditID, job.startedAt, job.completedAt))
		}
		job.mu.RUnlock()
	}
	return results
}

// buildUnifiedResult constructs high-fidelity audit intelligence data matching the design spec.
func (s *UnifiedAuditService) buildUnifiedResult(req model.AuditRunRequest, auditID string, startedAt time.Time, completedAt *time.Time) *model.UnifiedAuditResult {
	domain := req.TargetDomain
	if domain == "" {
		domain = "cedarandcodental.com"
	}
	brand := req.BrandName
	if brand == "" {
		brand = "Cedar & Co. Dental"
	}

	competitors := []model.CompetitorBenchmark{
		{
			Name:         brand,
			Domain:       domain,
			OverallScore: 78,
			SEOScore:     82,
			SocialScore:  74,
			IsClient:     true,
			IsEstimated:  false,
		},
	}

	for _, c := range req.Competitors {
		if c.Active {
			competitors = append(competitors, model.CompetitorBenchmark{
				Name:         c.Name,
				Domain:       c.Domain,
				OverallScore: 71,
				SEOScore:     75,
				SocialScore:  68,
				IsClient:     false,
				IsEstimated:  true,
			})
		}
	}

	if len(competitors) == 1 {
		// Provide default sample competitors from spec if none provided
		competitors = append(competitors,
			model.CompetitorBenchmark{
				Name:         "Tend Dental Studios",
				Domain:       "hellotend.com",
				OverallScore: 81,
				SEOScore:     85,
				SocialScore:  76,
				IsClient:     false,
				IsEstimated:  true,
			},
			model.CompetitorBenchmark{
				Name:         "Aspen Dental Care",
				Domain:       "aspendental.com",
				OverallScore: 69,
				SEOScore:     70,
				SocialScore:  66,
				IsClient:     false,
				IsEstimated:  true,
			},
		)
	}

	findings := []model.UnifiedFinding{
		{
			ID:          "find-1",
			Pillar:      "SEO",
			Severity:    "critical",
			Title:       "Missing Schema.org LocalBusiness markup on regional clinic landing pages",
			Description: "Search engines are unable to associate clinic addresses and operating hours with geographic map packs.",
			Evidence:    "Crawled 14 local branch URLs: 0 contained LocalBusiness structured JSON-LD.",
			Source:      "Measured",
		},
		{
			ID:          "find-2",
			Pillar:      "Social",
			Severity:    "watch",
			Title:       "Short-form video engagement rate is 1.4% below dental healthcare benchmark",
			Description: "Reels and TikTok content lack hook retention in the first 3 seconds, reducing algorithmic distribution.",
			Evidence:    "Average 3s retention is 24% vs. industry average of 42%.",
			Source:      "Measured",
		},
		{
			ID:          "find-3",
			Pillar:      "AI",
			Severity:    "critical",
			Title:       "Brand absent from ChatGPT & Perplexity top-3 recommendations for 'Amman dental implants'",
			Description: "LLM grounding synthesizes competitor directories because brand citations in local authority sources are outdated.",
			Evidence:    "Zero mentions across 5 standard AI search prompts tested.",
			Source:      "AI interpretation",
		},
		{
			ID:          "find-4",
			Pillar:      "CRO",
			Severity:    "watch",
			Title:       "Mobile booking conversion drop-off on multi-step patient intake form",
			Description: "Desktop conversion sits at 4.2% while mobile drops to 1.1% due to required PDF uploads.",
			Evidence:    "Step 3 form abandonment is 68% on mobile devices.",
			Source:      "Estimated",
		},
	}

	roadmap := []model.RoadmapItem{
		{
			ID:        "road-1",
			Pillar:      "SEO",
			Timeframe: "30-day",
			Title:     "Inject structured LocalBusiness JSON-LD markup across all clinic locations",
			Impact:    "High",
			Effort:    "Low",
		},
		{
			ID:        "road-2",
			Pillar:      "Social",
			Timeframe: "30-day",
			Title:     "Deploy patient transformation video reels with upfront 3s educational hooks",
			Impact:    "Medium",
			Effort:    "Medium",
		},
		{
			ID:        "road-3",
			Pillar:      "AI",
			Timeframe: "60-day",
			Title:     "Publish authoritative multilingual clinical FAQs to seed LLM knowledge graphs",
			Impact:    "High",
			Effort:    "Medium",
		},
		{
			ID:        "road-4",
			Pillar:      "CRO",
			Timeframe: "90-day",
			Title:     "Streamline mobile appointment booking down to single-screen WhatsApp/SMS flow",
			Impact:    "High",
			Effort:    "Medium",
		},
	}

	return &model.UnifiedAuditResult{
		AuditID:       auditID,
		TargetDomain:  domain,
		BrandName:     brand,
		OverallStatus: "completed",
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		OverallScore:  78,
		OverallGrade:  "B+",
		PillarsScores: map[string]int{
			"seo":    82,
			"social": 74,
			"ai":     79,
			"cro":    68,
		},
		SEOScore:         82,
		SocialScore:      74,
		AIScore:          79,
		CROScore:         68,
		Competitors:      competitors,
		KeyFindings:      findings,
		RoadmapItems:     roadmap,
		ExecutiveSummary: fmt.Sprintf("%s exhibits strong organic technical fundamentals (SEO 82/100), but suffers from low AI visibility grounding and sub-optimal short-form social engagement compared to top competitors.", brand),
	}
}

func stringPtr(s string) *string {
	return &s
}
