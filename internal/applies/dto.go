package applies

import (
	"time"

	"resume-backend/internal/generatedresumes"
)

// GeneratedResumeResponse is the outward-facing representation of a generated resume.
type GeneratedResumeResponse struct {
	GeneratedResumeID string    `json:"generatedResumeId"`
	DocumentID        string    `json:"documentId"`
	AnalysisID        string    `json:"analysisId"`
	TemplateID        string    `json:"templateId"`
	MimeType          string    `json:"mimeType"`
	SizeBytes         int64     `json:"sizeBytes"`
	CreatedAt         time.Time `json:"createdAt"`
}

func toGeneratedResumeResponse(resume generatedresumes.GeneratedResume) GeneratedResumeResponse {
	return GeneratedResumeResponse{
		GeneratedResumeID: resume.ID,
		DocumentID:        resume.DocumentID,
		AnalysisID:        resume.AnalysisID,
		TemplateID:        resume.TemplateID,
		MimeType:          resume.MimeType,
		SizeBytes:         resume.SizeBytes,
		CreatedAt:         resume.CreatedAt,
	}
}
