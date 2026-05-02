package analyses

import "time"

// AnalysisShare represents a share link for an analysis report.
type AnalysisShare struct {
	ID           string
	AnalysisID   string
	OwnerUserID  *string
	OwnerGuestID *string
	TokenHash    string
	TokenCipher  string
	CreatedAt    time.Time
	RevokedAt    *time.Time
}
