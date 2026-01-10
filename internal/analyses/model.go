package analyses

import "time"

// Analysis represents a document analysis job.
type Analysis struct {
	ID             string         `json:"id"`
	DocumentID     string         `json:"documentId"`
	UserID         string         `json:"userId"`
	JobDescription string         `json:"jobDescription"`
	PromptVersion  string         `json:"promptVersion"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Status         string         `json:"status"`
	Result         map[string]any `json:"result,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}
