package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"

	"github.com/voxlumedia/vox-social-backend/internal/config"
	"github.com/voxlumedia/vox-social-backend/internal/handler"
	"github.com/voxlumedia/vox-social-backend/internal/service"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Base context
	ctx := context.Background()

	// Initialize Storage Service (PostgreSQL pgxpool)
	storageSvc, err := service.NewStorageService(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer storageSvc.Close()

	// ── Social Audit Service (existing) ───────────────────────────────────
	aiSvc := service.NewAIService(
		cfg.AIApiKey,
		cfg.AIApiEndpoint,
		cfg.AIModel,
		cfg.ScraperApiKey,
		cfg.ScraperInstaEndpoint,
		cfg.ScraperFbEndpoint,
	)
	auditHandler := handler.NewAuditHandler(aiSvc, storageSvc)

	// ── SEO Audit Service (new) ───────────────────────────────────────────
	seoSvc := service.NewSEOService(service.SEOConfig{
		PageSpeedAPIKey:    cfg.PageSpeedAPIKey,
		AIApiKey:           cfg.AIApiKey,
		AIApiEndpoint:      cfg.AIApiEndpoint,
		AIModel:            cfg.AIModel,
		GooglePlacesAPIKey: cfg.GooglePlacesAPIKey,
		YelpAPIKey:         cfg.YelpAPIKey,
		DataForSEOLogin:    cfg.DataForSEOLogin,
		DataForSEOPassword: cfg.DataForSEOPassword,
		MaxSitemapPages:    cfg.SEOMaxSitemapPages,
		BrokenLinkLimit:    cfg.SEOBrokenLinkLimit,
	}, storageSvc)
	seoHandler := handler.NewSEOAuditHandler(seoSvc)

	// ── Auth Service (Firebase) ───────────────────────────────────────────
	// Since we only need to verify ID tokens, we can initialize without credentials.
	opt := option.WithoutAuthentication()
	firebaseApp, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.FirebaseProjectID}, opt)
	if err != nil {
		log.Fatalf("error initializing app: %v\n", err)
	}
	firebaseAuthClient, err := firebaseApp.Auth(ctx)
	if err != nil {
		log.Fatalf("error getting Auth client: %v\n", err)
	}

	authMiddleware := handler.NewAuthMiddleware(firebaseAuthClient)
	authHandler := handler.NewAuthHandler(storageSvc)

	// ── GoHighLevel Webhook Handler ───────────────────────────────────────
	ghlHandler := handler.NewGHLHandler(cfg.GHLWebhookURL)

	// ── Routes ────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Existing social audit
	mux.HandleFunc("/api/v1/analyze", corsMiddleware(auditHandler.HandleAnalyze, cfg.AllowedOrigins))

	// SEO audit
	mux.HandleFunc("/api/v1/seo/audit", corsMiddleware(seoHandler.HandleAudit, cfg.AllowedOrigins))
	mux.HandleFunc("/api/v1/seo/audit/", corsMiddleware(seoHandler.HandleGetAudit, cfg.AllowedOrigins))
	mux.HandleFunc("/api/v1/seo/domain/", corsMiddleware(seoHandler.HandleGetByDomain, cfg.AllowedOrigins))

	// Auth routes (protected by Firebase token verification)
	mux.HandleFunc("/api/v1/auth/sync", corsMiddleware(authMiddleware.RequireAuth(authHandler.HandleAuthSync), cfg.AllowedOrigins))

	// GHL webhook forwarding
	mux.HandleFunc("/api/v1/ghl/forward", corsMiddleware(ghlHandler.HandleForwardToGHL, cfg.AllowedOrigins))

	// Health check for Railway / uptime monitors
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Server configuration
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 165 * time.Second, // slightly above max audit timeout
		IdleTimeout:  180 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s...", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited properly")
}

// corsMiddleware unconditionally handles CORS checks for this public API.
func corsMiddleware(next http.HandlerFunc, allowedOrigins string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}
