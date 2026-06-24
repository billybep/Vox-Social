package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

	// Initialize AI Service
	aiSvc := service.NewAIService(
		cfg.AIApiKey,
		cfg.AIApiEndpoint,
		cfg.AIModel,
		cfg.ScraperApiKey,
		cfg.ScraperInstaEndpoint,
		cfg.ScraperFbEndpoint,
	)

	// Initialize Handlers
	auditHandler := handler.NewAuditHandler(aiSvc, storageSvc)

	// Set up routing
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/analyze", corsMiddleware(auditHandler.HandleAnalyze, cfg.AllowedOrigins))

	// Health check for Railway
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
		WriteTimeout: 150 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	// Graceful Shutdown Channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Run server in a goroutine
	go func() {
		log.Printf("Server starting on port %s...", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-stop
	log.Println("Shutting down server gracefully...")

	// Create a context with timeout for shutting down
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited properly")
}

// corsMiddleware unconditionally handles CORS checks for this public API
func corsMiddleware(next http.HandlerFunc, allowedOrigins string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Unconditionally allow origin for this public API
		if origin == "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")

		// Handle preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
