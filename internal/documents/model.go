package documents

import "time"

// Document represents an uploaded document owned by a user.
type Document struct {
	ID               string
	UserID           string
	FileName         string
	OriginalFilename string
	MimeType         string
	ContentType      string
	SizeBytes        int64
	StorageProvider  string
	StorageKey       string
	ExtractedTextKey string
	ExtractedAt      *time.Time
	CreatedAt        time.Time
}
