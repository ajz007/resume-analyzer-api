package util

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashUserKey returns a filesystem-safe identifier for a user ID.
func HashUserKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
