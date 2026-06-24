package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/voxlumedia/vox-social-backend/internal/model"
)

type AIService struct {
	AIApiKey             string
	AIEndpoint           string
	AIModel              string
	ScraperApiKey        string
	ScraperInstaEndpoint string
	ScraperFbEndpoint    string
	Client               *http.Client
}

func NewAIService(aiKey, aiEndpoint, aiModel, scraperKey, instaEndpoint, fbEndpoint string) *AIService {
	log.Printf("[INIT] AIService initialized")

	return &AIService{
		AIApiKey:             aiKey,
		AIEndpoint:           aiEndpoint,
		AIModel:              aiModel,
		ScraperApiKey:        scraperKey,
		ScraperInstaEndpoint: instaEndpoint,
		ScraperFbEndpoint:    fbEndpoint,
		Client: &http.Client{
			// No global timeout — let the context control it
			Timeout: 0,
		},
	}
}

// AnalyzeProfile orchestrates the scraping and AI analysis sequentially.
// The timeout is controlled by the caller's context (set in the handler).
func (s *AIService) AnalyzeProfile(ctx context.Context, profileURL string) (*model.AnalyzeResponse, error) {
	totalStart := time.Now()
	log.Printf("[ANALYZE] Starting analysis for: %s", profileURL)

	// 1. Fetch scraped data
	log.Printf("[ANALYZE] Step 1/2: Calling scraper...")
	scrapedText, err := s.scrapeProfile(ctx, profileURL)
	if err != nil {
		log.Printf("[ANALYZE] Step 1/2 FAILED after %v: %v", time.Since(totalStart), err)
		return nil, fmt.Errorf("scraping failed: %w", err)
	}
	log.Printf("[ANALYZE] Step 1/2 OK: Scraper returned %d bytes in %v", len(scrapedText), time.Since(totalStart))

	// 2. Optimize scraped data before sending to AI
	optimizedText := optimizeScrapedData(scrapedText)
	log.Printf("[ANALYZE] Optimization: reduced payload from %d to %d bytes", len(scrapedText), len(optimizedText))

	// 3. Send scraped data to AI
	aiStart := time.Now()
	log.Printf("[ANALYZE] Step 2/2: Calling AI...")
	result, err := s.callAI(ctx, optimizedText)
	if err != nil {
		log.Printf("[ANALYZE] Step 2/2 FAILED after %v (AI took %v): %v", time.Since(totalStart), time.Since(aiStart), err)
		return nil, err
	}
	log.Printf("[ANALYZE] Step 2/2 OK: AI responded in %v. Total: %v", time.Since(aiStart), time.Since(totalStart))

	return result, nil
}

func (s *AIService) scrapeProfile(ctx context.Context, profileURL string) (string, error) {
	var endpoint string
	var platform string
	if strings.Contains(strings.ToLower(profileURL), "instagram.com") {
		endpoint = s.ScraperInstaEndpoint
		platform = "Instagram"
	} else if strings.Contains(strings.ToLower(profileURL), "facebook.com") {
		endpoint = s.ScraperFbEndpoint
		platform = "Facebook"
	} else {
		return "", fmt.Errorf("unsupported platform. Only Facebook and Instagram URLs are supported")
	}

	log.Printf("[SCRAPER] Platform: %s | Endpoint: %s", platform, endpoint)

	// Build platform-specific Apify actor input
	var payload map[string]interface{}
	if platform == "Facebook" {
		payload = map[string]interface{}{
			"startUrls": []map[string]string{{"url": profileURL}},
		}
	} else {
		payload = map[string]interface{}{
			"directUrls":   []string{profileURL},
			"resultsType":  "posts",
			"resultsLimit": 12,
		}
	}
	log.Printf("[SCRAPER] Payload: %+v", payload)
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// Inject API Key via query param (Apify standard) AND Bearer header
	if s.ScraperApiKey != "" {
		q := req.URL.Query()
		q.Set("token", s.ScraperApiKey)
		req.URL.RawQuery = q.Encode()
	}

	start := time.Now()
	log.Printf("[SCRAPER] Sending request...")
	resp, err := s.Client.Do(req)
	if err != nil {
		log.Printf("[SCRAPER] HTTP request failed after %v: %v", time.Since(start), err)
		return "", fmt.Errorf("scraper HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[SCRAPER] Got response: status=%d in %v", resp.StatusCode, time.Since(start))

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read scraper response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		log.Printf("[SCRAPER] ERROR: status %d, body: %s", resp.StatusCode, string(bodyBytes[:min(500, len(bodyBytes))]))
		return "", fmt.Errorf("scraper returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[SCRAPER] Success: received %d bytes", len(bodyBytes))
	return string(bodyBytes), nil
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiRequest struct {
	Model          string            `json:"model"`
	Messages       []aiMessage       `json:"messages"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Temperature    float64           `json:"temperature"`
}

type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *AIService) callAI(ctx context.Context, scrapedText string) (*model.AnalyzeResponse, error) {
	systemPrompt := `You are an expert social media auditor. Analyze the provided scraped profile data. You MUST return ONLY a raw, valid JSON object without any markdown formatting or conversational text. The JSON must follow this exact structure:
{
  "overall_score": int (0-100),
  "status": string (POOR, GOOD, EXCELLENT),
  "profile_identity": string (brief summary),
  "growth_potential": string (Low, Moderate, High),
  "profile_readiness": int (0-100),
  "key_strengths": [array of 3 strings],
  "opportunities": [array of 3 strings],
  "recommended_package": string
}

CRITICAL RULE FOR 'recommended_package': 
You must evaluate the scraped profile data and choose exactly ONE package from the list below based strictly on which condition best matches the profile's current state. Output ONLY the exact package name:

- 'VoxLite Social Media Management': Recommend when profile is inactive, very low post frequency, just getting started on social.
- 'VoxBoost Social Media Management': Recommend when there is some posting activity but inconsistent — needs regular content calendar.
- 'VoxGrowth Social Media Management': Recommend when posting regularly but engagement is low — needs strategy and growth focus.
- 'VoxPro Social Media Management': Recommend when it is an established brand wanting full professional management across platforms.
- 'VoxMax Social Media Management': Recommend when it is a high-volume brand, all platforms, maximum content output needed.
- 'VOXCHAT AI Chat Automation': Recommend when DMs not automated, missing lead capture in comments or messages.
- 'VOXVOICE AI Voice Automation': Recommend when there is no voice follow-up system, missing phone automation.
- 'VOXREWARD Customer Loyalty Program': Recommend when no loyalty or retention program detected.
- 'VOXWEB Website Design & Development': Recommend when there is no website link in bio or website is weak/non-existent.`

	reqPayload := aiRequest{
		Model: s.AIModel,
		Messages: []aiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: "Raw scraped data:\n" + scrapedText},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
		Temperature:    0.2,
	}

	jsonBody, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	// Retry loop: up to 3 attempts for 503 (high demand) errors
	maxRetries := 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("[AI] Attempt %d/%d: Sending request to: %s (model: %s, payload: %d bytes)", attempt, maxRetries, s.AIEndpoint, s.AIModel, len(jsonBody))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.AIEndpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if s.AIApiKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.AIApiKey)
		}

		start := time.Now()
		resp, err := s.Client.Do(req)
		if err != nil {
			log.Printf("[AI] Attempt %d/%d: HTTP request failed after %v: %v", attempt, maxRetries, time.Since(start), err)
			return nil, fmt.Errorf("AI API request failed: %w", err)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[AI] Attempt %d/%d: Got response: status=%d in %v", attempt, maxRetries, resp.StatusCode, time.Since(start))

		// If 503, retry with exponential backoff
		if resp.StatusCode == http.StatusServiceUnavailable {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 2s, 4s, 8s
			log.Printf("[AI] Attempt %d/%d: 503 high demand. Retrying in %v...", attempt, maxRetries, backoff)
			lastErr = fmt.Errorf("AI API returned status 503: %s", string(bodyBytes[:min(200, len(bodyBytes))]))
			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return nil, fmt.Errorf("context canceled while waiting to retry: %w", ctx.Err())
			}
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[AI] ERROR: status %d, body: %s", resp.StatusCode, string(bodyBytes[:min(500, len(bodyBytes))]))
			return nil, fmt.Errorf("AI API returned status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var aiResp aiResponse
		if err := json.Unmarshal(bodyBytes, &aiResp); err != nil {
			return nil, fmt.Errorf("failed to decode AI response envelope: %w", err)
		}

		if len(aiResp.Choices) == 0 {
			return nil, fmt.Errorf("AI API returned no choices")
		}

		rawContent := aiResp.Choices[0].Message.Content

		var result model.AnalyzeResponse
		if err := json.Unmarshal([]byte(rawContent), &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal AI JSON output: %w | raw content: %s", err, rawContent)
		}

		return &result, nil
	}

	return nil, fmt.Errorf("AI API failed after %d retries: %w", maxRetries, lastErr)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// optimizeScrapedData filters the raw Apify JSON output to keep only the fields relevant
// for the AI audit, significantly reducing the payload size sent to the LLM.
func optimizeScrapedData(rawJSON string) string {
	var rawData []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &rawData); err != nil {
		// If it fails to parse as array, maybe it's a single object (e.g. facebook page profile)
		var singleObj map[string]interface{}
		if err2 := json.Unmarshal([]byte(rawJSON), &singleObj); err2 == nil {
			rawData = []map[string]interface{}{singleObj}
		} else {
			return rawJSON // Fallback to raw if parsing fails entirely
		}
	}

	var optimized []map[string]interface{}

	// Keys to keep that are relevant for AI analysis (includes bio, website, stats, captions)
	keepKeys := []string{
		"caption", "text", "message", "description",
		"likesCount", "commentsCount", "shares", "reactions", "videoPlayCount", "viewsCount",
		"timestamp", "date", "createdAt",
		"type", "postType", "mediaType",
		"ownerUsername", "username", "followersCount", "followsCount", "pageName", "likes", "followers",
		"bio", "biography", "website", "externalUrl", "url",
	}

	for _, item := range rawData {
		compactItem := make(map[string]interface{})
		
		// Recursively or just top-level? Apify usually puts bio/website at top-level or owner object.
		// Let's flatten top-level and 1-level deep for owner info if it exists.
		for k, v := range item {
			if k == "owner" || k == "page" || k == "user" {
				if subMap, ok := v.(map[string]interface{}); ok {
					for subK, subV := range subMap {
						if isKeyRelevant(subK, keepKeys) {
							compactItem[subK] = truncateIfString(subV)
						}
					}
				}
			}
			
			if isKeyRelevant(k, keepKeys) {
				compactItem[k] = truncateIfString(v)
			}
		}

		if len(compactItem) > 0 {
			optimized = append(optimized, compactItem)
		}
	}

	if len(optimized) == 0 {
		return rawJSON // Fallback if no known keys found
	}

	optBytes, err := json.Marshal(optimized)
	if err != nil {
		return rawJSON
	}

	return string(optBytes)
}

func isKeyRelevant(key string, keepKeys []string) bool {
	for _, k := range keepKeys {
		if strings.EqualFold(key, k) {
			return true
		}
	}
	return false
}

func truncateIfString(v interface{}) interface{} {
	if strVal, ok := v.(string); ok {
		if len(strVal) > 600 {
			return strVal[:600] + "..." // Truncate long captions to save tokens
		}
	}
	return v
}
