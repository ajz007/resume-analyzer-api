package usage

import "time"

const (
	ApplyRunStatusPlanned = "PLANNED"
	ApplyRunStatusDraft   = "DRAFT"
	ApplyRunStatusFinal   = "FINAL"
)

// ApplyRun tracks a resume apply execution attempt.
type ApplyRun struct {
	ID                    string
	UserID                string
	AnalysisID            string
	Status                string
	AutoFixesCount        int
	SafeRewritesCount     int
	BlockedRewritesCount  int
	NeedsInputCount       int
	PlaceholdersRemaining int
	DocumentVersionID     string
	CreatedAt             time.Time
}

// ApplyRunUpdate captures mutable fields of an apply run.
type ApplyRunUpdate struct {
	ID                    string
	UserID                string
	Status                string
	AutoFixesCount        int
	SafeRewritesCount     int
	BlockedRewritesCount  int
	NeedsInputCount       int
	PlaceholdersRemaining int
	DocumentVersionID     string
}

// DocumentVersion represents a rendered resume version.
type DocumentVersion struct {
	ID         string
	DocumentID string
	UserID     string
	ApplyRunID string
	FileName   string
	MimeType   string
	SizeBytes  int64
	StorageKey string
	CreatedAt  time.Time
}
