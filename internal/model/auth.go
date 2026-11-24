package model

import "time"

type User struct {
	ID           string    `json:"id"`
	UID          string    `json:"uid,omitempty"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName,omitempty"`
	Role         string    `json:"role,omitempty"`
	Credits      int       `json:"credits,omitempty"`
	PasswordHash *string   `json:"-"`
	GoogleID     *string   `json:"google_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type WorkspaceProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Credits int    `json:"credits"`
	Plan    string `json:"plan"`
}

type AuthSyncRequest struct {
	UID         string `json:"uid"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type AuthSyncResponse struct {
	Status    string           `json:"status"`
	User      User             `json:"user"`
	Workspace WorkspaceProfile `json:"workspace"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type GoogleLoginRequest struct {
	Credential string `json:"credential"` // The ID token from Google
}
