package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/voxlumedia/vox-social-backend/internal/model"
	"github.com/voxlumedia/vox-social-backend/internal/service"
)

type AuthHandler struct {
	storageSvc *service.StorageService
}

func NewAuthHandler(storageSvc *service.StorageService) *AuthHandler {
	return &AuthHandler{storageSvc: storageSvc}
}

// HandleAuthSync is called by the frontend after a successful Firebase login.
// It syncs user data (uid, email, displayName, role) and returns user + workspace profile.
func (h *AuthHandler) HandleAuthSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var syncReq model.AuthSyncRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&syncReq)
	}

	// Fallback to token claims in context if body fields are empty
	if syncReq.Email == "" {
		if email, ok := r.Context().Value(UserEmailKey).(string); ok {
			syncReq.Email = email
		}
	}
	if syncReq.UID == "" {
		if uid, ok := r.Context().Value(UserIDKey).(string); ok {
			syncReq.UID = uid
		}
	}

	syncReq.Email = strings.TrimSpace(syncReq.Email)
	syncReq.UID = strings.TrimSpace(syncReq.UID)

	if syncReq.Email == "" && syncReq.UID == "" {
		http.Error(w, "User identification (email or uid) required", http.StatusBadRequest)
		return
	}

	if syncReq.Role == "" {
		syncReq.Role = "agency_owner"
	}
	if syncReq.DisplayName == "" {
		if syncReq.Email != "" {
			syncReq.DisplayName = strings.Split(syncReq.Email, "@")[0]
		} else {
			syncReq.DisplayName = "User"
		}
	}

	// Try to get or persist user in DB if storage service is available
	var user *model.User
	var err error

	if h.storageSvc != nil && syncReq.Email != "" {
		user, err = h.storageSvc.GetUserByEmail(r.Context(), syncReq.Email)
		if err != nil {
			if err == pgx.ErrNoRows {
				user, err = h.storageSvc.CreateGoogleUser(r.Context(), syncReq.Email, syncReq.UID)
				if err != nil {
					log.Printf("Warning: failed to insert user into DB (%s): %v", syncReq.Email, err)
				} else {
					log.Printf("✨ New user registered: %s (%s)", syncReq.Email, syncReq.Role)
				}
			} else {
				log.Printf("Database error querying user (%s): %v", syncReq.Email, err)
			}
		}
	}

	// In-memory fallback if DB record wasn't created or DB is not reachable
	if user == nil {
		user = &model.User{
			ID:        syncReq.UID,
			UID:       syncReq.UID,
			Email:     syncReq.Email,
			CreatedAt: time.Now(),
		}
	}

	user.UID = syncReq.UID
	user.DisplayName = syncReq.DisplayName
	user.Role = syncReq.Role
	user.Credits = 50

	workspaceID := "ws_" + strings.ReplaceAll(user.ID, "-", "")
	if len(workspaceID) > 11 {
		workspaceID = workspaceID[:11]
	}

	workspace := model.WorkspaceProfile{
		ID:      workspaceID,
		Name:    syncReq.DisplayName + "'s Workspace",
		Role:    syncReq.Role,
		Credits: 50,
		Plan:    "agency_pro",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model.AuthSyncResponse{
		Status:    "synced",
		User:      *user,
		Workspace: workspace,
	})
}
