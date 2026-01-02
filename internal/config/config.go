package config

import "os"

// Config holds application configuration.
type Config struct {
	Port string
}

// Load reads configuration from environment variables.
func Load() Config {
	return Config{
		Port: getEnv("PORT", "8080"),
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
