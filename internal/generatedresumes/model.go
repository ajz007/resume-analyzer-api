package generatedresumes

import "time"

// GeneratedResume represents a stored resume generated from an analysis.
type GeneratedResume struct {
	ID         string
	UserID     string
	DocumentID string
	AnalysisID string
	TemplateID string
	StorageKey string
	MimeType   string
	SizeBytes  int64
	CreatedAt  time.Time
	DeletedAt  *time.Time
}
