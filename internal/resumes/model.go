package resumes

import (
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

const (
	StatusDraft      = "draft"
	SourceManual     = "manual"
	defaultListLimit = 20
	maxListLimit     = 100
)

type Resume struct {
	ID               string
	OwnerID          string
	Title            string
	Status           string
	CurrentVersionID string
	CurrentResume    modelv1.ResumeModel
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ResumeVersion struct {
	ID              string
	ResumeID        string
	VersionNumber   int
	SourceType      string
	Resume          modelv1.ResumeModel
	ChangeSummary   map[string]any
	ParentVersionID *string
	CreatedAt       time.Time
}

type SaveResult struct {
	Resume            Resume
	ReadinessWarnings []modelv1.ValidationWarning
}
