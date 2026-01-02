package config

import (
	"log"
	"os"
	"strings"
)

// Config holds application configuration.
type Config struct {
	Port            string
	CORSAllowOrigin []string
	LocalStoreDir   string
	DatabaseURL     string
	Env             string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	env := normalizeEnv(getEnv("ENV", "development"))
	dbURL := os.Getenv("DATABASE_URL")

	if env == "production" && dbURL == "" {
		log.Fatal("DATABASE_URL is required in production")
	}

	return Config{
		Port:            getEnv("PORT", "8080"),
		CORSAllowOrigin: splitAndTrim(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173")),
		LocalStoreDir:   getEnv("LOCAL_STORE_DIR", "./data"),
		DatabaseURL:     dbURL,
		Env:             env,
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
