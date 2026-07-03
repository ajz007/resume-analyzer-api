package resumes

import (
	"context"
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

const (
	GenerationJobStatusQueued     = "queued"
	GenerationJobStatusProcessing = "processing"
	GenerationJobStatusCompleted  = "completed"
	GenerationJobStatusFailed     = "failed"
)

type GenerationJob struct {
	ID           string
	OwnerID      string
	Status       string
	Request      GenerateRequest
	ResumeID     *string
	VersionID    *string
	Result       *GenerationJobResult
	ErrorType    *string
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type GenerationJobResult struct {
	RequiresUserInput []modelv1.RequiresUserInput `json:"requiresUserInput"`
	Assumptions       []modelv1.Assumption        `json:"assumptions"`
	Warnings          []modelv1.ResponseWarning   `json:"warnings"`
	GenerationMode    string                      `json:"generationMode,omitempty"`
	FallbackUsed      bool                        `json:"fallbackUsed"`
	FallbackReason    string                      `json:"fallbackReason,omitempty"`
	DraftType         string                      `json:"draftType,omitempty"`
}

type GenerationJobRepo interface {
	Create(ctx context.Context, job GenerationJob) error
	GetByID(ctx context.Context, jobID string) (GenerationJob, error)
	Update(ctx context.Context, job GenerationJob) error
	ClaimQueued(ctx context.Context, jobID string, startedAt time.Time) (GenerationJob, bool, error)
	MarkCompleted(ctx context.Context, jobID, resumeID, versionID string, result *GenerationJobResult, completedAt time.Time) (GenerationJob, bool, error)
	MarkFailed(ctx context.Context, jobID, errorType, errorMessage string, completedAt time.Time) (GenerationJob, bool, error)
}

func validGenerationJobStatus(status string) bool {
	switch status {
	case GenerationJobStatusQueued, GenerationJobStatusProcessing, GenerationJobStatusCompleted, GenerationJobStatusFailed:
		return true
	default:
		return false
	}
}
