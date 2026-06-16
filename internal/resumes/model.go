package resumes

import (
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

const (
	StatusDraft            = "draft"
	SourceManual           = "manual"
	SourceParsedFromUpload = "parsed_from_upload"
	SourceAIGenerated      = "ai_generated"
	SourceAIImproved       = "ai_improved"
	SourceAITailored       = "ai_tailored"
	SourceExportSnapshot   = "export_snapshot"
	maxTitleLength         = 160
	defaultListLimit       = 20
	maxListLimit           = 100
)

var allowedSourceTypes = map[string]struct{}{
	SourceManual:           {},
	SourceParsedFromUpload: {},
	SourceAIGenerated:      {},
	SourceAIImproved:       {},
	SourceAITailored:       {},
	SourceExportSnapshot:   {},
}

func validSourceType(sourceType string) bool {
	_, ok := allowedSourceTypes[sourceType]
	return ok
}

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

type ExportResult struct {
	FileName          string
	DocxBytes         []byte
	ReadinessWarnings []modelv1.ValidationWarning
}
