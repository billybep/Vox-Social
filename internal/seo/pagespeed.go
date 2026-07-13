package seo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// PageSpeed Insights
// ─────────────────────────────────────────────────────────────────────────────

type PSIResult struct {
	Available     bool
	Strategy      string
	Performance   int
	SEO           int
	Accessibility int
	BestPractices int
	LCP           string
	CLS           string
	INP           string
	FCP           string
	SpeedIndex    string
	TBT           string
	Error         string
}

// FetchPageSpeed calls the Google PageSpeed Insights API v5.
// Ported from extension's fetchPageSpeed().
func FetchPageSpeed(ctx context.Context, pageURL, strategy, apiKey string) PSIResult {
	params := url.Values{}
	params.Set("url", pageURL)
	params.Set("strategy", strategy)
	params.Add("category", "performance")
	params.Add("category", "seo")
	params.Add("category", "accessibility")
	params.Add("category", "best-practices")
	if apiKey != "" {
		params.Set("key", apiKey)
	}
	apiURL := "https://www.googleapis.com/pagespeedonline/v5/runPagespeed?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return PSIResult{Available: false, Strategy: strategy, Error: err.Error()}
	}

	res := FetchJSON(ctx, apiURL, req, 70*time.Second)
	if !res.OK || res.Text == "" {
		errMsg := "Service is temporarily unavailable. Please try again later."
		if res.Status == 429 {
			errMsg = "Daily analysis quota exceeded. Please try again later."
		} else {
			var errResp struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(res.Text), &errResp); err == nil && errResp.Error.Message != "" {
				errMsg = "API Error: " + errResp.Error.Message
			}
		}
		return PSIResult{Available: false, Strategy: strategy, Error: errMsg}
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(res.Text), &data); err != nil {
		return PSIResult{Available: false, Strategy: strategy, Error: "JSON parse error"}
	}

	catScore := func(cat string) int {
		cats, _ := data["lighthouseResult"].(map[string]interface{})
		if cats == nil {
			return 0
		}
		categories, _ := cats["categories"].(map[string]interface{})
		if categories == nil {
			return 0
		}
		c, _ := categories[cat].(map[string]interface{})
		if c == nil {
			return 0
		}
		s, ok := c["score"].(float64)
		if !ok {
			return 0
		}
		return int(s * 100)
	}

	metricDisplay := func(auditID string) string {
		lhr, _ := data["lighthouseResult"].(map[string]interface{})
		if lhr == nil {
			return ""
		}
		audits, _ := lhr["audits"].(map[string]interface{})
		if audits == nil {
			return ""
		}
		a, _ := audits[auditID].(map[string]interface{})
		if a == nil {
			return ""
		}
		dv, _ := a["displayValue"].(string)
		return dv
	}

	return PSIResult{
		Available:     true,
		Strategy:      strategy,
		Performance:   catScore("performance"),
		SEO:           catScore("seo"),
		Accessibility: catScore("accessibility"),
		BestPractices: catScore("best-practices"),
		LCP:           metricDisplay("largest-contentful-paint"),
		CLS:           metricDisplay("cumulative-layout-shift"),
		INP:           metricDisplay("interaction-to-next-paint"),
		FCP:           metricDisplay("first-contentful-paint"),
		SpeedIndex:    metricDisplay("speed-index"),
		TBT:           metricDisplay("total-blocking-time"),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// DataForSEO helpers (shared auth)
// ─────────────────────────────────────────────────────────────────────────────

func dataForSEOPost(ctx context.Context, endpoint string, payload interface{}, login, password string, timeout time.Duration) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(login, password)
	req.Header.Set("Content-Type", "application/json")

	res := FetchJSON(ctx, endpoint, req, timeout)
	if !res.OK {
		return nil, fmt.Errorf("DataForSEO error (status %d): %s", res.Status, truncate(res.Text, 200))
	}
	return []byte(res.Text), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
