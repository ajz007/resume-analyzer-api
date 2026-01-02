package config

import (
	"os"
	"strings"
)

// Config holds application configuration.
type Config struct {
	Port            string
	CORSAllowOrigin []string
	LocalStoreDir   string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		CORSAllowOrigin: splitAndTrim(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173")),
		LocalStoreDir:   getEnv("LOCAL_STORE_DIR", "./data"),
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
