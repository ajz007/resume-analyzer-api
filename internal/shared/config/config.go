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
	ObjectStoreType    string
	LocalStoreDir      string
	AWSRegion          string
	S3Bucket           string
	S3Prefix           string
	SSEKMSKeyID        string
	LLMProvider        string
	LLMModel           string
	AnalysisVersion    string
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
		log.Printf("DATABASE_URL is required in production")
	}

	return Config{
		Port:               getEnv("PORT", "8080"),
		CORSAllowOrigin:    splitAndTrim(getEnv("CORS_ALLOW_ORIGINS", "http://localhost:5173")),
		ObjectStoreType:    normalizeStoreType(getEnv("OBJECT_STORE", "local")),
		LocalStoreDir:      getEnv("LOCAL_STORE_DIR", "./data"),
		AWSRegion:          getEnv("AWS_REGION", ""),
		S3Bucket:           getEnv("S3_BUCKET", ""),
		S3Prefix:           getEnv("S3_PREFIX", ""),
		SSEKMSKeyID:        getEnv("SSE_KMS_KEY_ID", ""),
		LLMProvider:        getEnv("LLM_PROVIDER", "openai"),
		LLMModel:           getEnv("LLM_MODEL", ""),
		AnalysisVersion:    getEnv("ANALYSIS_VERSION", "gpt-5-mini:v1"),
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
	case "local":
		return "local"
	case "development", "dev":
		return "dev"
	default:
		return "dev"
	}
}

func normalizeStoreType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "s3":
		return "s3"
	default:
		return "local"
	}
}
