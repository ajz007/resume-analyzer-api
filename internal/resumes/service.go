package resumes

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	modelv1 "resume-backend/resume/modelv1"
	"resume-backend/resume/render"
)

type Service struct {
	Repo Repo
}

func (s *Service) Create(ctx context.Context, ownerID, title string, resume modelv1.ResumeModel) (SaveResult, error) {
	ownerID = strings.TrimSpace(ownerID)
	title = strings.TrimSpace(title)
	if ownerID == "" || title == "" {
		return SaveResult{}, ErrInvalidInput
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return SaveResult{}, ErrInvalidInput
	}
	if errs := modelv1.ValidateStructure(resume); len(errs) > 0 {
		return SaveResult{}, ValidationError{Errors: errs}
	}

	now := time.Now().UTC()
	resumeID := uuid.NewString()
	versionID := uuid.NewString()
	record := Resume{
		ID:               resumeID,
		OwnerID:          ownerID,
		Title:            title,
		Status:           StatusDraft,
		CurrentVersionID: versionID,
		CurrentResume:    resume,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	version := ResumeVersion{
		ID:            versionID,
		ResumeID:      resumeID,
		VersionNumber: 1,
		SourceType:    SourceManual,
		Resume:        resume,
		CreatedAt:     now,
	}

	created, err := s.Repo.Create(ctx, record, version)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{
		Resume:            created,
		ReadinessWarnings: modelv1.ValidateReadiness(resume),
	}, nil
}

func (s *Service) Update(ctx context.Context, ownerID, resumeID, title string, resume modelv1.ResumeModel, changeSummary map[string]any) (SaveResult, error) {
	ownerID = strings.TrimSpace(ownerID)
	resumeID = strings.TrimSpace(resumeID)
	title = strings.TrimSpace(title)
	if ownerID == "" || resumeID == "" || title == "" {
		return SaveResult{}, ErrInvalidInput
	}
	if utf8.RuneCountInString(title) > maxTitleLength {
		return SaveResult{}, ErrInvalidInput
	}
	if _, err := uuid.Parse(resumeID); err != nil {
		return SaveResult{}, ErrInvalidInput
	}
	if errs := modelv1.ValidateStructure(resume); len(errs) > 0 {
		return SaveResult{}, ValidationError{Errors: errs}
	}

	version := ResumeVersion{
		ID:            uuid.NewString(),
		ResumeID:      resumeID,
		SourceType:    SourceManual,
		Resume:        resume,
		ChangeSummary: changeSummary,
		CreatedAt:     time.Now().UTC(),
	}
	updated, err := s.Repo.Update(ctx, ownerID, resumeID, title, version)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{
		Resume:            updated,
		ReadinessWarnings: modelv1.ValidateReadiness(resume),
	}, nil
}

func (s *Service) Get(ctx context.Context, ownerID, resumeID string) (Resume, error) {
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(resumeID) == "" {
		return Resume{}, ErrInvalidInput
	}
	if _, err := uuid.Parse(resumeID); err != nil {
		return Resume{}, ErrInvalidInput
	}
	return s.Repo.GetByID(ctx, ownerID, resumeID)
}

func (s *Service) List(ctx context.Context, ownerID string, limit, offset int) ([]Resume, error) {
	if strings.TrimSpace(ownerID) == "" {
		return nil, ErrInvalidInput
	}
	return s.Repo.ListByOwner(ctx, ownerID, limit, offset)
}

func (s *Service) ListVersions(ctx context.Context, ownerID, resumeID string) ([]ResumeVersion, error) {
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(resumeID) == "" {
		return nil, ErrInvalidInput
	}
	if _, err := uuid.Parse(resumeID); err != nil {
		return nil, ErrInvalidInput
	}
	return s.Repo.ListVersions(ctx, ownerID, resumeID)
}

func (s *Service) GetVersion(ctx context.Context, ownerID, resumeID, versionID string) (ResumeVersion, error) {
	if strings.TrimSpace(ownerID) == "" || strings.TrimSpace(resumeID) == "" || strings.TrimSpace(versionID) == "" {
		return ResumeVersion{}, ErrInvalidInput
	}
	if _, err := uuid.Parse(resumeID); err != nil {
		return ResumeVersion{}, ErrInvalidInput
	}
	if _, err := uuid.Parse(versionID); err != nil {
		return ResumeVersion{}, ErrInvalidInput
	}
	return s.Repo.GetVersionByID(ctx, ownerID, resumeID, versionID)
}

func (s *Service) ExportDOCX(ctx context.Context, ownerID, resumeID string) (ExportResult, error) {
	resume, err := s.Get(ctx, ownerID, resumeID)
	if err != nil {
		return ExportResult{}, err
	}
	if errs := modelv1.ValidateStructure(resume.CurrentResume); len(errs) > 0 {
		return ExportResult{}, ValidationError{Errors: errs}
	}
	docxBytes, err := render.RenderResumeModelV1(resume.CurrentResume)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{
		FileName:          exportFileName(resume.Title),
		DocxBytes:         docxBytes,
		ReadinessWarnings: modelv1.ValidateReadiness(resume.CurrentResume),
	}, nil
}

func exportFileName(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "resume.docx"
	}
	var builder strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case r == ' ':
			builder.WriteRune('_')
		}
	}
	name := strings.Trim(builder.String(), "_-")
	if name == "" {
		name = "resume"
	}
	return name + ".docx"
}
