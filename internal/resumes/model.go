package resumes

import (
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

const (
	StatusDraft            = "draft"
	OriginBlank            = "blank"
	OriginManual           = "manual"
	OriginParsedFromUpload = "parsed_from_upload"
	OriginAIGenerated      = "ai_generated"
	OriginAITailored       = "ai_tailored"
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

var allowedOriginTypes = map[string]struct{}{
	OriginBlank:            {},
	OriginManual:           {},
	OriginParsedFromUpload: {},
	OriginAIGenerated:      {},
	OriginAITailored:       {},
}

func validSourceType(sourceType string) bool {
	_, ok := allowedSourceTypes[sourceType]
	return ok
}

func validOriginType(originType string) bool {
	if originType == "" {
		return true
	}
	_, ok := allowedOriginTypes[originType]
	return ok
}

type Resume struct {
	ID                   string
	OwnerID              string
	Title                string
	Status               string
	SourceResumeID       string
	SourceVersionID      string
	OriginType           string
	CurrentVersionID     string
	CurrentResume        modelv1.ResumeModel
	CurrentChangeSummary map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ResumeVersion struct {
	ID              string
	ResumeID        string
	VersionNumber   int
	SourceType      string
	SourceVersionID string
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
	FileName  string
	DocxBytes []byte
}
