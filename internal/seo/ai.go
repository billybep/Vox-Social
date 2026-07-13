package seo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

type AIRecommendations struct {
	Available        bool
	Source           string
	ExecutiveSummary []string
	ActionPlan       []ActionPlanItem
	Reason           string
}

// ─────────────────────────────────────────────────────────────────────────────
// OpenAI Recommendations
// ─────────────────────────────────────────────────────────────────────────────

// AuditSnapshot is the data sent to OpenAI for AI-generated recommendations.
type AuditSnapshot struct {
	URL          string
	BusinessName string
	Keyword      string
	Location     string
	OverallScore int
	Grade        string
	Scores       map[string]int
	TopIssues    []Check
	PSIMobile    *int
	PSIDesktop   *int
	BrokenLinks  int
}

// FetchAIRecommendations calls an OpenAI-compatible API to generate
// an executive summary and action plan. Falls back to rule-based if no key.
func FetchAIRecommendations(ctx context.Context, snapshot AuditSnapshot, apiKey, model, endpoint string) AIRecommendations {
	if apiKey == "" {
		return AIRecommendations{Available: false, Reason: "AI API key not configured. Rule-based recommendations were used."}
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}

	type issuePayload struct {
		Category string `json:"category"`
		Title    string `json:"title"`
		Status   string `json:"status"`
		Impact   string `json:"impact"`
		Details  string `json:"details"`
		Fix      string `json:"fix"`
	}
	issueList := make([]issuePayload, 0, len(snapshot.TopIssues))
	for _, c := range snapshot.TopIssues {
		issueList = append(issueList, issuePayload{c.Category, c.Title, c.Status, c.Impact, c.Details, c.Fix})
	}

	advanced := map[string]interface{}{
		"pageSpeedMobile":  snapshot.PSIMobile,
		"pageSpeedDesktop": snapshot.PSIDesktop,
		"brokenLinks":      snapshot.BrokenLinks,
	}

	userContent, _ := json.Marshal(map[string]interface{}{
		"task":                  "Write a professional SEO audit executive summary and 30-day action plan for this report. Return JSON with executiveSummary array of 6 concise bullets and actionPlan array of 8 objects with title and description.",
		"website":               snapshot.URL,
		"businessName":          snapshot.BusinessName,
		"targetKeyword":         snapshot.Keyword,
		"targetLocation":        snapshot.Location,
		"overallScore":          snapshot.OverallScore,
		"grade":                 snapshot.Grade,
		"scores":                snapshot.Scores,
		"highestPriorityChecks": issueList,
		"advanced":              advanced,
	})

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a senior local SEO auditor. Return only valid JSON. No markdown. Keep wording client-friendly, professional, and action-oriented."},
			{"role": "user", "content": string(userContent)},
		},
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return AIRecommendations{Available: false, Reason: "Failed to create request: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	res := FetchJSON(ctx, endpoint, req, 65*time.Second)
	if !res.OK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		var geminiErrArr []struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		errMsg := "Unknown API error"
		if err := json.Unmarshal([]byte(res.Text), &errResp); err == nil && errResp.Error.Message != "" {
			errMsg = errResp.Error.Message
		} else if err := json.Unmarshal([]byte(res.Text), &geminiErrArr); err == nil && len(geminiErrArr) > 0 && geminiErrArr[0].Error.Message != "" {
			errMsg = geminiErrArr[0].Error.Message
		} else {
			errMsg = truncate(res.Text, 120)
		}
		return AIRecommendations{Available: false, Reason: fmt.Sprintf("AI Service Unavailable (Status %d): %s", res.Status, errMsg)}
	}

	// Parse standard OpenAI chat completion response format
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(res.Text), &openAIResp); err != nil {
		return AIRecommendations{Available: false, Reason: "Failed to decode AI response"}
	}

	if len(openAIResp.Choices) == 0 {
		return AIRecommendations{Available: false, Reason: "Empty AI response"}
	}

	text := strings.TrimSpace(openAIResp.Choices[0].Message.Content)

	// Extract JSON object from text (strip markdown blocks if present)
	if !strings.HasPrefix(text, "{") {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			text = text[start : end+1]
		}
	}

	var parsed struct {
		ExecutiveSummary []string `json:"executiveSummary"`
		ActionPlan       []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"actionPlan"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return AIRecommendations{Available: false, Reason: "AI response was not valid JSON. Rule-based recommendations were used."}
	}

	// Cap at reasonable lengths
	execSum := parsed.ExecutiveSummary
	if len(execSum) > 8 {
		execSum = execSum[:8]
	}
	var actionPlan []ActionPlanItem
	for _, a := range parsed.ActionPlan {
		if a.Title != "" {
			actionPlan = append(actionPlan, ActionPlanItem{Title: a.Title, Description: a.Description})
		}
		if len(actionPlan) >= 10 {
			break
		}
	}

	return AIRecommendations{
		Available:        true,
		Source:           "AI Recommendation Engine",
		ExecutiveSummary: execSum,
		ActionPlan:       actionPlan,
	}
}
