package generatedresumes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/documents"
	"resume-backend/internal/shared/storage/object"
	resumeservice "resume-backend/resume/service"
)

const analysisStatusCompleted = "completed"

// AnalysisRecord contains the analysis fields needed to generate a resume.
type AnalysisRecord struct {
	ID         string
	UserID     string
	DocumentID string
	Status     string
	Result     map[string]any
}

// AnalysisReader loads analysis records for generated resumes.
type AnalysisReader interface {
	GetAnalysisByID(ctx context.Context, analysisID string) (AnalysisRecord, error)
}

// Service contains business logic for generated resumes.
type Service struct {
	Repo         Repo
	AnalysisRepo AnalysisReader
	DocRepo      documents.DocumentsRepo
	Store        object.ObjectStore
}

// CreateFromAnalysis generates and stores a resume from the analysis results.
func (s *Service) CreateFromAnalysis(ctx context.Context, userID, analysisID, templateID string) (GeneratedResume, error) {
	if userID == "" || analysisID == "" || templateID == "" {
		return GeneratedResume{}, ErrInvalidInput
	}
	if s.Repo == nil || s.AnalysisRepo == nil || s.DocRepo == nil || s.Store == nil {
		return GeneratedResume{}, errors.New("missing dependencies")
	}

	analysis, err := s.AnalysisRepo.GetAnalysisByID(ctx, analysisID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return GeneratedResume{}, ErrNotFound
		}
		return GeneratedResume{}, err
	}
	if analysis.UserID != userID {
		return GeneratedResume{}, ErrNotFound
	}
	if analysis.Status != analysisStatusCompleted || analysis.Result == nil {
		return GeneratedResume{}, ErrInvalidInput
	}

	doc, err := s.DocRepo.GetByID(ctx, userID, analysis.DocumentID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			return GeneratedResume{}, ErrNotFound
		}
		return GeneratedResume{}, err
	}

	reader, err := s.Store.Open(ctx, doc.StorageKey)
	if err != nil {
		return GeneratedResume{}, err
	}
	defer reader.Close()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return GeneratedResume{}, err
	}

	result, err := decodeAnalysisResult(analysis.Result)
	if err != nil {
		return GeneratedResume{}, ErrInvalidInput
	}

	execResult, err := resumeservice.ExecuteApply(ctx, string(raw), result, resumeservice.ApplyHeaderInputs{})
	if err != nil {
		return GeneratedResume{}, err
	}

	fileName := "resume_generated_" + templateID + ".docx"
	storageKey, size, mimeType, err := s.Store.Save(ctx, userID, fileName, bytes.NewReader(execResult.DocxBytes))
	if err != nil {
		return GeneratedResume{}, err
	}

	resume := GeneratedResume{
		ID:         uuid.NewString(),
		UserID:     userID,
		DocumentID: doc.ID,
		AnalysisID: analysis.ID,
		TemplateID: templateID,
		StorageKey: storageKey,
		MimeType:   mimeType,
		SizeBytes:  size,
		CreatedAt:  time.Now().UTC(),
	}

	if err := s.Repo.Create(ctx, resume); err != nil {
		return GeneratedResume{}, err
	}
	return resume, nil
}

// Get returns a generated resume by ID for a user.
func (s *Service) Get(ctx context.Context, userID, generatedResumeID string) (GeneratedResume, error) {
	if userID == "" || generatedResumeID == "" {
		return GeneratedResume{}, ErrInvalidInput
	}
	return s.Repo.GetByID(ctx, userID, generatedResumeID)
}

// List returns generated resumes for a user ordered newest-first.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]GeneratedResume, error) {
	if userID == "" {
		return nil, ErrInvalidInput
	}
	return s.Repo.ListByUser(ctx, userID, limit, offset)
}

func decodeAnalysisResult(result map[string]any) (resumeservice.AnalysisResultV2_3, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return resumeservice.AnalysisResultV2_3{}, err
	}
	var decoded resumeservice.AnalysisResultV2_3
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return resumeservice.AnalysisResultV2_3{}, err
	}
	return decoded, nil
}
