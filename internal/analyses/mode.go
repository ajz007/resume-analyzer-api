package analyses

import (
	"errors"
	"strings"
)

// AnalysisMode defines the supported analysis modes.
type AnalysisMode string

const (
	ModeATS      AnalysisMode = "ATS"
	ModeJobMatch AnalysisMode = "JOB_MATCH"
)

// ParseMode normalizes and validates a mode string.
func ParseMode(raw string) (AnalysisMode, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", errors.New("analysis mode is required")
	}
	switch strings.ToUpper(normalized) {
	case string(ModeATS):
		return ModeATS, nil
	case string(ModeJobMatch):
		return ModeJobMatch, nil
	default:
		return "", errors.New("analysis mode is invalid")
	}
}
