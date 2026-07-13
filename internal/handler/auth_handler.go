package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/voxlumedia/vox-social-backend/internal/service"
)

type AuthHandler struct {
	storageSvc *service.StorageService
}

func NewAuthHandler(storageSvc *service.StorageService) *AuthHandler {
	return &AuthHandler{storageSvc: storageSvc}
}

// HandleAuthSync is called by the frontend after a successful Firebase login.
// It ensures that the user exists in our local PostgreSQL 'users' table.
func (h *AuthHandler) HandleAuthSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email, ok := r.Context().Value(UserEmailKey).(string)
	if !ok || email == "" {
		http.Error(w, "Email missing in token claims", http.StatusBadRequest)
		return
	}
	
	uid, ok := r.Context().Value(UserIDKey).(string)
	if !ok || uid == "" {
		http.Error(w, "UID missing in token", http.StatusBadRequest)
		return
	}

	// Try to get existing user by email
	user, err := h.storageSvc.GetUserByEmail(r.Context(), email)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Create user in our DB (treating uid from Firebase as the GoogleID/AuthID for now)
			user, err = h.storageSvc.CreateGoogleUser(r.Context(), email, uid)
			if err != nil {
				log.Printf("Error creating user %s: %v", email, err)
				http.Error(w, "Failed to sync user: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("✨ New user registered via Firebase: %s", email)
		} else {
			log.Printf("Database error syncing user %s: %v", email, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	} else {
		log.Printf("✅ User logged in: %s", email)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "synced",
		"user":   user,
	})
}

