package util

import (
	"errors"
	"strings"
)

// SanitizeFileName removes path separators and rejects traversal patterns.
func SanitizeFileName(name string) (string, error) {
	if strings.Contains(name, "..") {
		return "", errors.New("invalid file name")
	}
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	if s == "" {
		return "", errors.New("invalid file name")
	}
	return s, nil
}
