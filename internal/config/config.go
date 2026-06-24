package config

import (
	"os"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	AllowedOrigins       string
	AIApiKey             string
	AIApiEndpoint        string
	AIModel              string
	ScraperApiKey        string
	ScraperInstaEndpoint string
	ScraperFbEndpoint    string
}

func LoadConfig() *Config {
	return &Config{
		Port:                 getEnv("PORT", "8080"),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/vox_social?sslmode=disable"),
		AllowedOrigins:       getEnv("ALLOWED_ORIGINS", "*"),
		AIApiKey:             getEnv("AI_API_KEY", ""),
		AIApiEndpoint:        getEnv("AI_API_ENDPOINT", "https://api.groq.com/openai/v1/chat/completions"),
		AIModel:              getEnv("AI_MODEL", "llama3-70b-8192"),
		ScraperApiKey:        getEnv("SCRAPER_API_KEY", ""),
		ScraperInstaEndpoint: getEnv("SCRAPER_INSTA_ENDPOINT", "https://api.apify.com/v2/acts/apify~instagram-scraper/run-sync-get-dataset-items"),
		ScraperFbEndpoint:    getEnv("SCRAPER_FB_ENDPOINT", "https://api.apify.com/v2/acts/apify~facebook-pages-scraper/run-sync-get-dataset-items"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
