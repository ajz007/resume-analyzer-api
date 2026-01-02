package config

import (
	"log"
	"os"
	"strings"
)

// Config holds application configuration.
type Config struct {
	Port        string
	DatabaseURL string
	Env         string
}

// Load reads configuration from environment variables.
func Load() Config {
	env := normalizeEnv(getEnv("ENV", "development"))
	dbURL := os.Getenv("DATABASE_URL")

	if env == "production" && dbURL == "" {
		log.Fatal("DATABASE_URL is required in production")
	}

	return Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: dbURL,
		Env:         env,
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func normalizeEnv(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "production", "prod":
		return "production"
	case "staging":
		return "staging"
	default:
		return "development"
	}
}
