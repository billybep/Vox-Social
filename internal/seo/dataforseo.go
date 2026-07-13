package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Rankings
// ─────────────────────────────────────────────────────────────────────────────

type RankingsResult struct {
	Available bool
	Keyword   string
	Location  string
	Matched   bool
	Position  int
	TopResults []RankingEntry
	Reason    string
	Error     string
}

type RankingEntry struct {
	Rank   int    `json:"rank"`
	Title  string `json:"title"`
	Domain string `json:"domain"`
	URL    string `json:"url"`
}

// FetchDataForSEORankings queries DataForSEO SERP for a keyword and checks if
// the target domain appears in the results.
func FetchDataForSEORankings(ctx context.Context, request AuditRequest, login, password string) RankingsResult {
	if login == "" || password == "" {
		return RankingsResult{Available: false, Reason: "DataForSEO credentials not configured."}
	}
	if request.TargetKeyword == "" {
		return RankingsResult{Available: false, Reason: "Target keyword not provided."}
	}
	location := request.TargetLocation
	if location == "" {
		location = "United States"
	}

	payload := []map[string]interface{}{{
		"keyword":       request.TargetKeyword,
		"location_name": location,
		"language_code": "en",
		"device":        "desktop",
		"os":            "windows",
	}}

	body, err := dataForSEOPost(ctx,
		"https://api.dataforseo.com/v3/serp/google/organic/live/advanced",
		payload, login, password, 45*time.Second)
	if err != nil {
		return RankingsResult{Available: false, Reason: err.Error()}
	}

	var resp struct {
		Tasks []struct {
			Result []struct {
				Items []struct {
					RankGroup    int    `json:"rank_group"`
					RankAbsolute int    `json:"rank_absolute"`
					Title        string `json:"title"`
					Domain       string `json:"domain"`
					URL          string `json:"url"`
				} `json:"items"`
			} `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Tasks) == 0 {
		return RankingsResult{Available: false, Reason: "DataForSEO response parse error"}
	}

	items := resp.Tasks[0].Result[0].Items
	targetDomain := DomainFromURL(request.WebsiteURL)
	matched := false
	position := 0
	for _, it := range items {
		d := it.Domain
		if len(d) > 4 && d[:4] == "www." {
			d = d[4:]
		}
		if d == targetDomain {
			matched = true
			position = it.RankGroup
			if position == 0 {
				position = it.RankAbsolute
			}
			break
		}
	}

	top := items
	if len(top) > 5 {
		top = top[:5]
	}
	topResults := make([]RankingEntry, 0, len(top))
	for _, it := range top {
		r := it.RankGroup
		if r == 0 {
			r = it.RankAbsolute
		}
		topResults = append(topResults, RankingEntry{Rank: r, Title: it.Title, Domain: it.Domain, URL: it.URL})
	}

	return RankingsResult{
		Available:   true,
		Keyword:     request.TargetKeyword,
		Location:    location,
		Matched:     matched,
		Position:    position,
		TopResults:  topResults,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Backlinks
// ─────────────────────────────────────────────────────────────────────────────

type BacklinksResult struct {
	Available         bool
	Target            string
	Backlinks         int
	ReferringDomains  int
	Rank              int
	BrokenBacklinks   int
	Dofollow          int
	Nofollow          int
	Reason            string
}

// FetchDataForSEOBacklinks retrieves a backlink summary from DataForSEO.
func FetchDataForSEOBacklinks(ctx context.Context, request AuditRequest, login, password string) BacklinksResult {
	if login == "" || password == "" {
		return BacklinksResult{Available: false, Reason: "DataForSEO credentials not configured."}
	}
	target := DomainFromURL(request.WebsiteURL)
	payload := []map[string]interface{}{{
		"target":               target,
		"internal_list_limit":  10,
		"backlinks_status_type": "live",
	}}

	body, err := dataForSEOPost(ctx,
		"https://api.dataforseo.com/v3/backlinks/summary/live",
		payload, login, password, 45*time.Second)
	if err != nil {
		return BacklinksResult{Available: false, Reason: err.Error()}
	}

	var resp struct {
		Tasks []struct {
			Result []struct {
				Backlinks        int `json:"backlinks"`
				ReferringDomains int `json:"referring_domains"`
				Rank             int `json:"rank"`
				BrokenBacklinks  int `json:"broken_backlinks"`
				Dofollow         int `json:"dofollow"`
				Nofollow         int `json:"nofollow"`
			} `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return BacklinksResult{Available: false, Reason: fmt.Sprintf("parse error: %v", err)}
	}
	if len(resp.Tasks) == 0 || len(resp.Tasks[0].Result) == 0 {
		return BacklinksResult{Available: false, Reason: "Backlink summary unavailable."}
	}

	r := resp.Tasks[0].Result[0]
	return BacklinksResult{
		Available:        true,
		Target:           target,
		Backlinks:        r.Backlinks,
		ReferringDomains: r.ReferringDomains,
		Rank:             r.Rank,
		BrokenBacklinks:  r.BrokenBacklinks,
		Dofollow:         r.Dofollow,
		Nofollow:         r.Nofollow,
	}
}
