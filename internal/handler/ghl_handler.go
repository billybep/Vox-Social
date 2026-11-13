package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

type GHLHandler struct {
	webhookURL string
}

func NewGHLHandler(webhookURL string) *GHLHandler {
	return &GHLHandler{
		webhookURL: webhookURL,
	}
}

func (h *GHLHandler) HandleForwardToGHL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	// Parse the incoming JSON payload
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("Error decoding payload: %v", err)
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Marshal the payload back to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling payload: %v", err)
		http.Error(w, "Error processing payload", http.StatusInternalServerError)
		return
	}

	// Forward to GHL webhook
	req, err := http.NewRequest(http.MethodPost, h.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error creating request to GHL: %v", err)
		http.Error(w, "Error creating request to GHL", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error forwarding to GHL: %v", err)
		http.Error(w, "Error forwarding to GHL", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response from GHL
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response from GHL: %v", err)
	}

	if resp.StatusCode >= 400 {
		log.Printf("GHL webhook returned error status %d: %s", resp.StatusCode, string(respBody))
		http.Error(w, "GHL webhook error", resp.StatusCode)
		return
	}

	log.Printf("Successfully forwarded payload to GHL")

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"Payload forwarded to GoHighLevel"}`))
}
