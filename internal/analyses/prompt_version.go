package analyses

import (
	"log"
	"os"
	"strings"
)

const DefaultPromptVersion = "v2_3"

const analysisPromptVersionEnv = "ANALYSIS_PROMPT_VERSION"

func NormalizePromptVersion(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return trimmed
	}
	envVersion := strings.TrimSpace(os.Getenv(analysisPromptVersionEnv))
	if envVersion == "" {
		return DefaultPromptVersion
	}
	if isValidPromptVersion(envVersion) {
		return envVersion
	}
	log.Printf("invalid %s=%q, falling back to %s", analysisPromptVersionEnv, envVersion, DefaultPromptVersion)
	return DefaultPromptVersion
}

func isValidPromptVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case "v1", "v2", "v2_1", "v2_2", "v2_3", "v2_4":
		return true
	default:
		return false
	}
}
