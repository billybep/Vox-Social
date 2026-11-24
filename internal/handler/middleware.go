package handler

import (
	"context"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
)

// AuthContextKey is the key for the authenticated user ID in the context
type AuthContextKey string

const UserIDKey AuthContextKey = "user_id"
const UserEmailKey AuthContextKey = "user_email"

type AuthMiddleware struct {
	authClient *auth.Client
}

func NewAuthMiddleware(authClient *auth.Client) *AuthMiddleware {
	return &AuthMiddleware{
		authClient: authClient,
	}
}

// RequireAuth is a middleware that verifies the Firebase ID token in the Authorization header.
// If the request is for /api/v1/auth/sync without an Authorization header or during local dev,
// it passes through so HandleAuthSync can validate the JSON request body.
func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// For /api/v1/auth/sync, allow reading UID and email directly from the JSON body
			if strings.Contains(r.URL.Path, "/auth/sync") {
				next(w, r)
				return
			}
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			if strings.Contains(r.URL.Path, "/auth/sync") {
				next(w, r)
				return
			}
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		// Verify the token
		token, err := m.authClient.VerifyIDToken(r.Context(), tokenString)
		if err != nil {
			// If verification fails but this is /auth/sync, let the handler check the body payload (e.g. dev mock token)
			if strings.Contains(r.URL.Path, "/auth/sync") {
				next(w, r)
				return
			}
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// Pass the user information in the context
		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, token.UID)
		
		if email, ok := token.Claims["email"].(string); ok {
			ctx = context.WithValue(ctx, UserEmailKey, email)
		}

		next(w, r.WithContext(ctx))
	}
}
