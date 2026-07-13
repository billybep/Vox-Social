package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Google Places
// ─────────────────────────────────────────────────────────────────────────────

type GooglePlaceResult struct {
	Available bool
	PlaceID   string
	Name      string
	Address   string
	Phone     string
	Website   string
	Rating    float64
	Reviews   int
	Status    string
	Reason    string
	Error     string
}

// FetchGooglePlace looks up a business via the Google Places Find + Details API.
// Ported from extension's fetchGooglePlace().
func FetchGooglePlace(ctx context.Context, request AuditRequest, apiKey string) GooglePlaceResult {
	if apiKey == "" {
		return GooglePlaceResult{Available: false, Reason: "Google Places API key not configured."}
	}
	parts := []string{request.BusinessName, request.TargetLocation, request.Phone}
	var filtered []string
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	query := strings.Join(filtered, " ")
	if query == "" {
		return GooglePlaceResult{Available: false, Reason: "Business name/location/phone not provided."}
	}

	findURL := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/findplacefromtext/json?input=%s&inputtype=textquery&fields=place_id,name,formatted_address,rating,user_ratings_total,business_status&key=%s",
		url.QueryEscape(query), url.QueryEscape(apiKey),
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, findURL, nil)
	res := FetchJSON(ctx, findURL, req, 25*time.Second)
	if !res.OK {
		return GooglePlaceResult{Available: false, Reason: "Google Places Find API error: " + res.Error}
	}

	var findResp struct {
		Candidates []struct {
			PlaceID string `json:"place_id"`
			Name    string `json:"name"`
			Rating  float64 `json:"rating"`
			Reviews int    `json:"user_ratings_total"`
			Status  string `json:"business_status"`
		} `json:"candidates"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal([]byte(res.Text), &findResp); err != nil || len(findResp.Candidates) == 0 {
		reason := "No Google place match found."
		if findResp.ErrorMessage != "" {
			reason = findResp.ErrorMessage
		}
		return GooglePlaceResult{Available: false, Reason: reason}
	}

	candidate := findResp.Candidates[0]
	detailsURL := fmt.Sprintf(
		"https://maps.googleapis.com/maps/api/place/details/json?place_id=%s&fields=name,formatted_address,formatted_phone_number,website,url,rating,user_ratings_total,business_status&key=%s",
		url.QueryEscape(candidate.PlaceID), url.QueryEscape(apiKey),
	)
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, detailsURL, nil)
	res2 := FetchJSON(ctx, detailsURL, req2, 25*time.Second)

	var detailResp struct {
		Result struct {
			Name    string  `json:"name"`
			Address string  `json:"formatted_address"`
			Phone   string  `json:"formatted_phone_number"`
			Website string  `json:"website"`
			Rating  float64 `json:"rating"`
			Reviews int     `json:"user_ratings_total"`
			Status  string  `json:"business_status"`
		} `json:"result"`
		ErrorMessage string `json:"error_message"`
	}
	if res2.OK && json.Unmarshal([]byte(res2.Text), &detailResp) == nil {
		r := detailResp.Result
		return GooglePlaceResult{
			Available: true,
			PlaceID:   candidate.PlaceID,
			Name:      r.Name,
			Address:   r.Address,
			Phone:     r.Phone,
			Website:   r.Website,
			Rating:    r.Rating,
			Reviews:   r.Reviews,
			Status:    r.Status,
		}
	}
	return GooglePlaceResult{
		Available: true,
		PlaceID:   candidate.PlaceID,
		Name:      candidate.Name,
		Rating:    candidate.Rating,
		Reviews:   candidate.Reviews,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Yelp Fusion
// ─────────────────────────────────────────────────────────────────────────────

type YelpBusiness struct {
	Name       string
	URL        string
	Phone      string
	Rating     float64
	Reviews    int
	Address    string
	Categories string
}

type YelpResult struct {
	Available  bool
	Businesses []YelpBusiness
	Reason     string
}

// FetchYelpBusiness searches Yelp Fusion for a matching business.
// Ported from extension's fetchYelpBusiness().
func FetchYelpBusiness(ctx context.Context, request AuditRequest, apiKey string) YelpResult {
	if apiKey == "" {
		return YelpResult{Available: false, Reason: "Yelp Fusion API key not configured."}
	}
	term := request.BusinessName
	if term == "" {
		term = DomainFromURL(request.WebsiteURL)
	}
	location := request.TargetLocation
	if location == "" {
		location = "United States"
	}

	yelpURL := fmt.Sprintf(
		"https://api.yelp.com/v3/businesses/search?term=%s&location=%s&limit=3",
		url.QueryEscape(term), url.QueryEscape(location),
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, yelpURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res := FetchJSON(ctx, yelpURL, req, 25*time.Second)
	if !res.OK {
		return YelpResult{Available: false, Reason: "Yelp API error: " + res.Error}
	}

	var yelpResp struct {
		Businesses []struct {
			Name        string  `json:"name"`
			URL         string  `json:"url"`
			Phone       string  `json:"phone"`
			DisplayPhone string `json:"display_phone"`
			Rating      float64 `json:"rating"`
			ReviewCount int     `json:"review_count"`
			Location    struct {
				DisplayAddress []string `json:"display_address"`
			} `json:"location"`
			Categories []struct {
				Title string `json:"title"`
			} `json:"categories"`
		} `json:"businesses"`
	}
	if err := json.Unmarshal([]byte(res.Text), &yelpResp); err != nil || len(yelpResp.Businesses) == 0 {
		return YelpResult{Available: false, Reason: "No Yelp match found."}
	}

	var businesses []YelpBusiness
	for _, b := range yelpResp.Businesses {
		phone := b.DisplayPhone
		if phone == "" {
			phone = b.Phone
		}
		addr := strings.Join(b.Location.DisplayAddress, ", ")
		var cats []string
		for _, c := range b.Categories {
			cats = append(cats, c.Title)
		}
		businesses = append(businesses, YelpBusiness{
			Name:       b.Name,
			URL:        b.URL,
			Phone:      phone,
			Rating:     b.Rating,
			Reviews:    b.ReviewCount,
			Address:    addr,
			Categories: strings.Join(cats, ", "),
		})
	}
	return YelpResult{Available: true, Businesses: businesses}
}
