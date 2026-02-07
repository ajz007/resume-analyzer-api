package analyses

import "time"

// Analysis represents a document analysis job.
type Analysis struct {
	ID                  string         `json:"id"`
	DocumentID          string         `json:"documentId"`
	UserID              string         `json:"userId"`
	JobDescription      string         `json:"jobDescription"`
	PromptVersion       string         `json:"promptVersion"`
	Mode                AnalysisMode   `json:"mode"`
	AnalysisVersion     string         `json:"analysisVersion"`
	PromptHash          string         `json:"promptHash"`
	Provider            string         `json:"provider"`
	Model               string         `json:"model"`
	ErrorCode           string         `json:"errorCode,omitempty"`
	ErrorMessage        *string        `json:"errorMessage,omitempty"`
	ErrorRetryable      bool           `json:"retryable,omitempty"`
	StartedAt           *time.Time     `json:"startedAt,omitempty"`
	CompletedAt         *time.Time     `json:"completedAt,omitempty"`
	AnalysisCompletedAt *time.Time     `json:"analysisCompletedAt,omitempty"`
	Status              string         `json:"status"`
	Result              map[string]any `json:"result,omitempty"`
	AnalysisRaw         any            `json:"-"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
}
