package applies

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/analyses"
	"resume-backend/internal/documents"
	"resume-backend/internal/generatedresumes"
	"resume-backend/internal/llm"
	"resume-backend/internal/shared/storage/object"
	"resume-backend/resume/model"
	"resume-backend/resume/render"
)

const defaultTemplateID = "resume_modern_ats_v1"

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrAnalysisNotComplete = errors.New("analysis not complete")
	ErrMissingExtracted    = errors.New("extracted text missing")
	ErrInvalidLLMOutput    = errors.New("invalid llm output")
	ErrInvalidResumeModel  = errors.New("invalid resume model")
)

// LLMClient generates ResumeModel payloads from prompts.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Service coordinates the apply pipeline.
type Service struct {
	AnalysisRepo  analyses.Repo
	DocumentsRepo documents.DocumentsRepo
	GeneratedRepo generatedresumes.Repo
	Store         object.ObjectStore
	LLM           LLMClient
}

// Apply generates, renders, and stores a resume for an analysis.
func (s *Service) Apply(ctx context.Context, userID, analysisID, templateID string) (generatedresumes.GeneratedResume, error) {
	if userID == "" || analysisID == "" {
		return generatedresumes.GeneratedResume{}, ErrInvalidInput
	}
	if templateID == "" {
		templateID = defaultTemplateID
	}
	if templateID != defaultTemplateID {
		return generatedresumes.GeneratedResume{}, ErrInvalidInput
	}
	if s.AnalysisRepo == nil || s.DocumentsRepo == nil || s.GeneratedRepo == nil || s.Store == nil || s.LLM == nil {
		return generatedresumes.GeneratedResume{}, errors.New("missing dependencies")
	}

	analysis, err := s.AnalysisRepo.GetByID(ctx, analysisID)
	if err != nil {
		if errors.Is(err, analyses.ErrNotFound) {
			return generatedresumes.GeneratedResume{}, ErrNotFound
		}
		return generatedresumes.GeneratedResume{}, err
	}
	if analysis.UserID != userID {
		return generatedresumes.GeneratedResume{}, ErrNotFound
	}
	if analysis.Status != analyses.StatusCompleted || analysis.Result == nil {
		return generatedresumes.GeneratedResume{}, ErrAnalysisNotComplete
	}

	doc, err := s.DocumentsRepo.GetByID(ctx, userID, analysis.DocumentID)
	if err != nil {
		if errors.Is(err, documents.ErrNotFound) {
			return generatedresumes.GeneratedResume{}, ErrNotFound
		}
		return generatedresumes.GeneratedResume{}, err
	}
	if strings.TrimSpace(doc.ExtractedTextKey) == "" {
		return generatedresumes.GeneratedResume{}, ErrMissingExtracted
	}

	extracted, err := loadText(ctx, s.Store, doc.ExtractedTextKey)
	if err != nil {
		return generatedresumes.GeneratedResume{}, err
	}

	prompt, err := buildPrompt(extracted, analysis.Result)
	if err != nil {
		return generatedresumes.GeneratedResume{}, err
	}

	raw, err := s.LLM.Complete(ctx, prompt)
	if err != nil {
		return generatedresumes.GeneratedResume{}, err
	}

	jsonPayload, err := extractJSONObject(raw)
	if err != nil {
		log.Printf("apply pipeline invalid json analysis_id=%s: %v", analysis.ID, err)
		return generatedresumes.GeneratedResume{}, ErrInvalidLLMOutput
	}

	var resumeModel model.ResumeModel
	if err := json.Unmarshal([]byte(jsonPayload), &resumeModel); err != nil {
		log.Printf("apply pipeline decode failed analysis_id=%s: %v", analysis.ID, err)
		return generatedresumes.GeneratedResume{}, ErrInvalidLLMOutput
	}

	if err := validateResumeModel(resumeModel); err != nil {
		return generatedresumes.GeneratedResume{}, ErrInvalidResumeModel
	}

	docxBytes, err := render.RenderResume(resumeModel)
	if err != nil {
		return generatedresumes.GeneratedResume{}, err
	}

	fileName := "resume_generated_" + templateID + ".docx"
	storageKey, size, mimeType, err := s.Store.Save(ctx, userID, fileName, bytes.NewReader(docxBytes))
	if err != nil {
		return generatedresumes.GeneratedResume{}, err
	}

	resume := generatedresumes.GeneratedResume{
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
	if err := s.GeneratedRepo.Create(ctx, resume); err != nil {
		return generatedresumes.GeneratedResume{}, err
	}
	return resume, nil
}

func loadText(ctx context.Context, store object.ObjectStore, key string) (string, error) {
	reader, err := store.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildPrompt(resumeText string, analysisResult map[string]any) (string, error) {
	analysisJSON, err := json.Marshal(analysisResult)
	if err != nil {
		return "", err
	}
	template := llm.ResumeGenPromptV1()
	replacer := strings.NewReplacer(
		"{{RESUME_TEXT}}", resumeText,
		"{{ANALYSIS_JSON}}", string(analysisJSON),
	)
	return replacer.Replace(template), nil
}

func extractJSONObject(raw string) (string, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return "", errors.New("empty llm response")
	}
	if json.Valid([]byte(payload)) {
		return payload, nil
	}

	start := strings.Index(payload, "{")
	end := strings.LastIndex(payload, "}")
	if start == -1 || end == -1 || end <= start {
		return "", errors.New("no json object found")
	}

	candidate := payload[start : end+1]
	if !json.Valid([]byte(candidate)) {
		return "", errors.New("invalid json object")
	}
	return candidate, nil
}

func validateResumeModel(resumeModel model.ResumeModel) error {
	if strings.TrimSpace(resumeModel.Header.Name) == "" {
		return ErrInvalidResumeModel
	}
	if len(resumeModel.Summary) > 4 {
		return ErrInvalidResumeModel
	}
	for _, exp := range resumeModel.Experience {
		if len(exp.Highlights) > 5 {
			return ErrInvalidResumeModel
		}
	}
	return nil
}
