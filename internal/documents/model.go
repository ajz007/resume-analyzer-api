package documents

import "time"

// Document represents an uploaded document owned by a user.
type Document struct {
	ID         string
	UserID     string
	FileName   string
	MimeType   string
	SizeBytes  int64
	StorageKey string
	CreatedAt  time.Time
}
