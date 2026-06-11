package config

import (
	"os"
)

type Config struct {
	Port           string
	DatabaseURL    string
	AllowedOrigins string
	AIApiKey       string
	AIApiEndpoint  string
}

func LoadConfig() *Config {
	return &Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:password@localhost:5432/vox_social?sslmode=disable"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:3000,https://vox-social.netlify.app"),
		AIApiKey:       getEnv("AI_API_KEY", ""),
		AIApiEndpoint:  getEnv("AI_API_ENDPOINT", "https://api.example.com/v1/analyze"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
