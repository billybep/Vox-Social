package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	AllowedOrigins       string

	// Auth
	FirebaseProjectID    string

	// Social audit (existing)
	AIApiKey             string
	AIApiEndpoint        string
	AIModel              string
	ScraperApiKey        string
	ScraperInstaEndpoint string
	ScraperFbEndpoint    string

	// SEO audit
	PageSpeedAPIKey    string
	OpenAIAPIKey       string
	OpenAIModel        string
	GooglePlacesAPIKey string
	YelpAPIKey         string
	DataForSEOLogin    string
	DataForSEOPassword string
	SEOMaxSitemapPages int
	SEOBrokenLinkLimit int
}

func LoadConfig() *Config {
	return &Config{
		Port:                 getEnv("PORT", "8080"),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/vox_social?sslmode=disable"),
		AllowedOrigins:       getEnv("ALLOWED_ORIGINS", "*"),
		FirebaseProjectID:    getEnv("FIREBASE_PROJECT_ID", "your-firebase-project-id"),
		AIApiKey:             getEnv("AI_API_KEY", ""),
		AIApiEndpoint:        getEnv("AI_API_ENDPOINT", "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"),
		AIModel:              getEnv("AI_MODEL", "gemini-2.5-flash-lite"),
		ScraperApiKey:        getEnv("SCRAPER_API_KEY", ""),
		ScraperInstaEndpoint: getEnv("SCRAPER_INSTA_ENDPOINT", "https://api.apify.com/v2/acts/apify~instagram-scraper/run-sync-get-dataset-items"),
		ScraperFbEndpoint:    getEnv("SCRAPER_FB_ENDPOINT", "https://api.apify.com/v2/acts/apify~facebook-pages-scraper/run-sync-get-dataset-items"),

		// SEO audit — all optional, gracefully degrade when empty
		PageSpeedAPIKey:    getEnv("PAGESPEED_API_KEY", ""),
		OpenAIAPIKey:       getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:        getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		GooglePlacesAPIKey: getEnv("GOOGLE_PLACES_API_KEY", ""),
		YelpAPIKey:         getEnv("YELP_API_KEY", ""),
		DataForSEOLogin:    getEnv("DATAFORSEO_LOGIN", ""),
		DataForSEOPassword: getEnv("DATAFORSEO_PASSWORD", ""),
		SEOMaxSitemapPages: getEnvInt("SEO_MAX_SITEMAP_PAGES", 200),
		SEOBrokenLinkLimit: getEnvInt("SEO_BROKEN_LINK_LIMIT", 60),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return fallback
}
