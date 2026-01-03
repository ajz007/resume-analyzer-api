package config

import (
	"log"
	"os"
	"strings"
)

// Config holds application configuration.
type Config struct {
	Port               string
	CORSAllowOrigin    []string
	LocalStoreDir      string
	DatabaseURL        string
	Env                string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	UIRedirectURL      string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	// Best-effort load of local env files for dev convenience.
	loadEnvFiles(".env", "cmd/.env")

	env := normalizeEnv(getEnv("ENV", "dev"))
	dbURL := os.Getenv("DATABASE_URL")

	if env == "production" && dbURL == "" {
		log.Fatal("DATABASE_URL is required in production")
	}

	return Config{
		Port:               getEnv("PORT", "8080"),
		CORSAllowOrigin:    splitAndTrim(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173")),
		LocalStoreDir:      getEnv("LOCAL_STORE_DIR", "./data"),
		DatabaseURL:        dbURL,
		Env:                env,
		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
		UIRedirectURL:      getEnv("UI_REDIRECT_URL", ""),
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
	case "development", "dev":
		return "dev"
	default:
		return "dev"
	}
}
