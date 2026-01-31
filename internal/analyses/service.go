package analyses

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/documents"
	"resume-backend/internal/extract"
	"resume-backend/internal/llm"
	"resume-backend/internal/queue"
	"resume-backend/internal/shared/metrics"
	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/shared/telemetry"
	"resume-backend/internal/usage"
)

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Service contains business logic for analyses.
type Service struct {
	Repo            Repo
	Usage           *usage.Service
	DocRepo         documents.DocumentsRepo
	Store           object.ObjectStore
	LLM             llm.Client
	JobQueue        queue.Client
	Provider        string
	Model           string
	AnalysisVersion string
}

// Create enqueues a new analysis and kicks off asynchronous completion.
func (s *Service) Create(ctx context.Context, documentID, userID, jobDescription, promptVersion string) (Analysis, error) {
	if documentID == "" || userID == "" {
		return Analysis{}, errors.New("documentID and userID are required")
	}
	if promptVersion == "" {
		promptVersion = "v2_3"
	}

	if s.Usage != nil {
		ok, _, err := s.Usage.CanConsume(ctx, userID, 1)
		if err != nil {
			return Analysis{}, err
		}
		if !ok {
			return Analysis{}, usage.ErrLimitReached
		}
	}

	analysis := Analysis{
		ID:              uuid.NewString(),
		DocumentID:      documentID,
		UserID:          userID,
		JobDescription:  jobDescription,
		PromptVersion:   promptVersion,
		AnalysisVersion: normalizeAnalysisVersion(s.AnalysisVersion),
		Provider:        normalizeProvider(s.Provider),
		Model:           s.Model,
		Status:          StatusQueued,
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.Repo.Create(ctx, analysis); err != nil {
		return Analysis{}, err
	}

	if s.Usage != nil {
		if _, err := s.Usage.Consume(ctx, userID, 1); err != nil {
			return Analysis{}, err
		}
	}

	if s.JobQueue == nil {
		return Analysis{}, ErrJobQueueNotConfigured
	}
	if err := s.JobQueue.Send(ctx, queue.Message{
		AnalysisID: analysis.ID,
		RequestID:  requestIDFromContext(ctx),
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Version:    1,
	}); err != nil {
		return Analysis{}, err
	}

	return analysis, nil
}

// StartOrReuse enqueues a new analysis or reuses an existing one for idempotent requests.
func (s *Service) StartOrReuse(ctx context.Context, documentID, userID, jobDescription, promptVersion string, allowRetry bool) (Analysis, bool, error) {
	if documentID == "" || userID == "" {
		return Analysis{}, false, errors.New("documentID and userID are required")
	}
	if promptVersion == "" {
		promptVersion = "v2_3"
	}

	analysis := Analysis{
		ID:              uuid.NewString(),
		DocumentID:      documentID,
		UserID:          userID,
		JobDescription:  jobDescription,
		PromptVersion:   promptVersion,
		AnalysisVersion: normalizeAnalysisVersion(s.AnalysisVersion),
		Provider:        normalizeProvider(s.Provider),
		Model:           s.Model,
		Status:          StatusQueued,
		CreatedAt:       time.Now().UTC(),
	}

	var allowCreate func() error
	if s.Usage != nil {
		allowCreate = func() error {
			ok, _, err := s.Usage.CanConsume(ctx, userID, 1)
			if err != nil {
				return err
			}
			if !ok {
				return usage.ErrLimitReached
			}
			return nil
		}
	}

	createdAnalysis, created, err := s.Repo.GetOrCreateForDocument(ctx, analysis, allowRetry, allowCreate)
	if err != nil {
		return createdAnalysis, false, err
	}
	if created && s.Usage != nil {
		if _, err := s.Usage.Consume(ctx, userID, 1); err != nil {
			return createdAnalysis, false, err
		}
	}
	if created {
		if s.JobQueue == nil {
			return createdAnalysis, created, ErrJobQueueNotConfigured
		}
		if err := s.JobQueue.Send(ctx, queue.Message{
			AnalysisID: createdAnalysis.ID,
			RequestID:  requestIDFromContext(ctx),
			EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Version:    1,
		}); err != nil {
			return createdAnalysis, created, err
		}
	}
	return createdAnalysis, created, nil
}

// Get returns an analysis by ID.
func (s *Service) Get(ctx context.Context, analysisID string) (Analysis, error) {
	if analysisID == "" {
		return Analysis{}, errors.New("analysisID is required")
	}
	return s.Repo.GetByID(ctx, analysisID)
}

// List returns analyses for a user ordered newest-first.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]Analysis, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	return s.Repo.ListByUser(ctx, userID, limit, offset)
}

func normalizeProvider(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "openai"
	}
	return provider
}

func normalizeAnalysisVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return "unknown"
	}
	return strings.TrimSpace(version)
}

func normalizeStorageProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "s3":
		return "s3"
	case "db", "local":
		return "local"
	default:
		return "local"
	}
}

// ProcessAnalysis executes analysis processing synchronously.
func (s *Service) ProcessAnalysis(ctx context.Context, analysisID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			s.failAnalysis(ctx, analysisID, "", "", err, nil)
		}
	}()

	analysis, err := s.Repo.GetByID(ctx, analysisID)
	if err != nil {
		err = fmt.Errorf("analysis lookup: %w", err)
		s.failAnalysis(ctx, analysisID, "", "", err, nil)
		return err
	}
	if analysis.Status == StatusCompleted || analysis.Status == StatusFailed {
		return nil
	}

	startedAt := time.Now().UTC()
	if err := s.Repo.UpdateStatusResultAndError(ctx, analysisID, StatusProcessing, nil, nil, nil, nil, &startedAt, nil); err != nil {
		// THIS is the bug you're currently hiding
		err = fmt.Errorf("set processing failed: %w", err)
		s.failAnalysis(ctx, analysisID, "", "", err, &startedAt)
		return err
	}

	metrics.IncAnalysisStarted()
	telemetry.Info("analysis.status", map[string]any{
		"request_id":        requestIDFromContext(ctx),
		"user_id":           analysis.UserID,
		"document_id":       analysis.DocumentID,
		"analysis_id":       analysis.ID,
		"status":            StatusProcessing,
		"status_transition": "queued->processing",
	})
	if s.DocRepo == nil || s.Store == nil {
		err = errors.New("missing document store dependencies")
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}
	if s.LLM == nil {
		err = errors.New("missing llm client")
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}
	requestID := requestIDFromContext(ctx)
	llmClient := newRetryingLLM(s.LLM, analysisID, requestID)

	doc, err := s.DocRepo.GetByID(ctx, analysis.UserID, analysis.DocumentID)
	if err != nil {
		err = fmt.Errorf("document lookup id=%s: %w", analysis.DocumentID, err)
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}
	storageProvider := normalizeStorageProvider(doc.StorageProvider)
	telemetry.Info("analysis.document.storage", map[string]any{
		"request_id":       requestID,
		"document_id":      doc.ID,
		"storage_provider": storageProvider,
	})

	extractedKey := doc.ExtractedTextKey
	var extracted string
	if extractedKey == "" {
		switch storageProvider {
		case "s3":
			s3Client, err := newS3DocClient(ctx)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: s3 client: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			raw, err := s3Client.GetObjectBytes(ctx, doc.StorageKey)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: s3 read: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			extracted, err = extract.ExtractTextFromBytes(ctx, raw, doc.MimeType, doc.FileName)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			extractedKey = doc.StorageKey + ".extracted.txt"
			if err := s3Client.PutText(ctx, extractedKey, extracted); err != nil {
				err = fmt.Errorf("document %s mime %s: store extracted: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			if err := s.DocRepo.UpdateExtraction(ctx, doc.UserID, doc.ID, extractedKey, time.Now().UTC()); err != nil {
				err = fmt.Errorf("document %s mime %s: update extraction: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
		default:
			if _, err := extract.ExtractText(ctx, s.Store, doc.StorageKey, doc.MimeType, doc.FileName); err != nil {
				err = fmt.Errorf("document %s mime %s: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			extractedKey = doc.StorageKey + ".extracted.txt"
			if err := s.DocRepo.UpdateExtraction(ctx, doc.UserID, doc.ID, extractedKey, time.Now().UTC()); err != nil {
				err = fmt.Errorf("document %s mime %s: update extraction: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
		}
	}

	if extracted == "" {
		switch storageProvider {
		case "s3":
			s3Client, err := newS3DocClient(ctx)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: s3 client: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			raw, err := s3Client.GetObjectBytes(ctx, extractedKey)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: load extracted text: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			extracted = string(raw)
		default:
			var err error
			extracted, err = loadText(ctx, s.Store, extractedKey)
			if err != nil {
				err = fmt.Errorf("document %s mime %s: load extracted text: %w", doc.ID, doc.MimeType, err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
		}
	}

	input := llm.AnalyzeInput{
		ResumeText:     extracted,
		JobDescription: analysis.JobDescription,
		PromptVersion:  analysis.PromptVersion,
		TargetRole:     "",
	}
	var promptHash string
	ctxWithHash := llm.WithPromptHashCapture(ctx, &promptHash)

	var raw json.RawMessage
	if analysis.PromptVersion == "v2" {
		raw, err = ValidateV2WithRetry(ctxWithHash, llmClient, input)
		if err != nil {
			err = fmt.Errorf("llm validate v2: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
		if err := s.storeAnalysisRaw(ctx, analysisID, raw); err != nil {
			err = fmt.Errorf("set analysis raw failed: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
	} else if analysis.PromptVersion == "v2_2" {
		raw, err = ValidateV2_2WithRetry(ctxWithHash, llmClient, input)
		if err != nil {
			err = fmt.Errorf("llm validate v2_2: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
		if err := s.storeAnalysisRaw(ctx, analysisID, raw); err != nil {
			err = fmt.Errorf("set analysis raw failed: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
	} else if analysis.PromptVersion == "v2_3" {
		raw, err = ValidateV2_3WithRetry(ctxWithHash, llmClient, input)
		if err != nil {
			err = fmt.Errorf("llm validate v2_3: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
		if err := s.storeAnalysisRaw(ctx, analysisID, raw); err != nil {
			err = fmt.Errorf("set analysis raw failed: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
	} else {
		raw, err = llmClient.AnalyzeResume(ctxWithHash, input)
		if err != nil {
			err = fmt.Errorf("llm analyze: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
		if err := s.storeAnalysisRaw(ctx, analysisID, raw); err != nil {
			err = fmt.Errorf("set analysis raw failed: %w", err)
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}

		var parsed AnalysisResultV1
		if err := json.Unmarshal(raw, &parsed); err != nil {
			rawRetry, retryErr := llmClient.AnalyzeResume(llm.WithFixJSON(ctxWithHash, string(raw)), input)
			if retryErr != nil {
				err = fmt.Errorf("llm analyze retry: %w", retryErr)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			if err := json.Unmarshal(rawRetry, &parsed); err != nil {
				if storeErr := s.storeAnalysisRaw(ctx, analysisID, rawRetry); storeErr != nil {
					err = fmt.Errorf("set analysis raw failed: %w", storeErr)
					s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
					return err
				}
				err = fmt.Errorf("llm output invalid: %w", err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			raw = rawRetry
			if err := s.storeAnalysisRaw(ctx, analysisID, raw); err != nil {
				err = fmt.Errorf("set analysis raw failed: %w", err)
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
		}
	}
	if promptHash == "" {
		// TODO: Ensure prompt_hash is captured for non-OpenAI providers if/when added.
		promptHash = ""
	}
	if err := s.Repo.UpdatePromptMetadata(ctx, analysisID, analysis.AnalysisVersion, promptHash); err != nil {
		err = fmt.Errorf("set prompt metadata failed: %w", err)
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}

	result, err := normalizeAnalysisResult(raw, analysis)
	if err != nil {
		err = fmt.Errorf("llm output invalid: %w", err)
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}

	completedAt := time.Now().UTC()
	if err := s.Repo.UpdateAnalysisResult(ctx, analysisID, result, &completedAt); err != nil {
		err = fmt.Errorf("set analysis result failed: %w", err)
		s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
		return err
	}
	metrics.IncAnalysisCompleted()
	metrics.ObserveAnalysisDurationMs(durationMs(&startedAt, &completedAt))
	telemetry.Info("analysis.status", map[string]any{
		"request_id":        requestIDFromContext(ctx),
		"user_id":           analysis.UserID,
		"document_id":       analysis.DocumentID,
		"analysis_id":       analysis.ID,
		"status":            StatusCompleted,
		"status_transition": "processing->completed",
		"duration_ms":       durationMs(&startedAt, &completedAt),
	})
	return nil
}

func (s *Service) completeAsync(ctx context.Context, analysisID string) {
	_ = s.ProcessAnalysis(ctx, analysisID)
}

func (s *Service) failAnalysis(ctx context.Context, analysisID, userID, documentID string, err error, startedAt *time.Time) {
	code, retryable := classifyFailure(err)
	msg := sanitizeError(err)
	completedAt := time.Now().UTC()
	if updateErr := s.Repo.UpdateStatusResultAndError(context.Background(), analysisID, StatusFailed, nil, &code, &msg, &retryable, nil, &completedAt); updateErr != nil {
		fmt.Printf("failAnalysis: update failed id=%s err=%v orig=%v\n", analysisID, updateErr, err)
	}
	metrics.IncAnalysisFailed()
	if startedAt != nil {
		metrics.ObserveAnalysisDurationMs(durationMs(startedAt, &completedAt))
	}
	telemetry.Info("analysis.status", map[string]any{
		"request_id":        requestIDFromContext(ctx),
		"user_id":           userID,
		"document_id":       documentID,
		"analysis_id":       analysisID,
		"status":            StatusFailed,
		"status_transition": "processing->failed",
		"duration_ms":       durationMs(startedAt, &completedAt),
	})
}

func durationMs(startedAt, completedAt *time.Time) float64 {
	if startedAt == nil || completedAt == nil {
		return 0
	}
	return float64(completedAt.Sub(*startedAt).Microseconds()) / 1000.0
}

func classifyFailure(err error) (string, bool) {
	if err == nil {
		return ErrorCodeInternal, false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCodeLLMTimeout, true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "openai request timeout") {
		return ErrorCodeLLMTimeout, true
	}
	if strings.Contains(msg, "timeout") && strings.Contains(msg, "llm") {
		return ErrorCodeLLMTimeout, true
	}
	if strings.Contains(msg, "schema") || strings.Contains(msg, "llm output invalid") || strings.Contains(msg, "llm output parse") {
		return ErrorCodeLLMSchemaMismatch, false
	}
	if strings.Contains(msg, "llm validate") || strings.Contains(msg, "llm output") {
		return ErrorCodeLLMSchemaMismatch, false
	}
	if strings.Contains(msg, "validation") && !strings.Contains(msg, "llm") {
		return ErrorCodeValidation, false
	}
	if strings.Contains(msg, "document") || strings.Contains(msg, "storage") || strings.Contains(msg, "analysis raw") || strings.Contains(msg, "analysis result") || strings.Contains(msg, "prompt metadata") || strings.Contains(msg, "set processing") {
		return ErrorCodeStorage, true
	}
	return ErrorCodeInternal, false
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	msg = strings.TrimSpace(msg)
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	return msg
}

func loadText(ctx context.Context, store object.ObjectStore, key string) (string, error) {
	body, err := store.Open(ctx, key)
	if err != nil {
		return "", err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildRawPayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{"rawText": ""}
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return parsed
	}
	return map[string]any{"rawText": string(raw)}
}

func (s *Service) storeAnalysisRaw(ctx context.Context, analysisID string, raw json.RawMessage) error {
	rawPayload := buildRawPayload(raw)
	return s.Repo.UpdateAnalysisRaw(ctx, analysisID, rawPayload)
}

func normalizeResultOrdering(result map[string]any) {
	if result == nil {
		return
	}

	if atsRaw, ok := result["ats"]; ok {
		if ats, ok := atsRaw.(map[string]any); ok {
			normalizeStringArray(ats, "formattingIssues")
			if mkRaw, ok := ats["missingKeywords"]; ok {
				if mk, ok := mkRaw.(map[string]any); ok {
					normalizeStringArray(mk, "fromJobDescription")
					normalizeStringArray(mk, "industryCommon")
				}
			}
		}
	}

	normalizeStringArray(result, "missingInformation")

	if planRaw, ok := result["actionPlan"]; ok {
		if plan, ok := planRaw.(map[string]any); ok {
			normalizeStringArray(plan, "quickWins")
			normalizeStringArray(plan, "mediumEffort")
			normalizeStringArray(plan, "deepFixes")
		}
	}
}

func normalizeStringArray(container map[string]any, key string) {
	raw, ok := container[key]
	if !ok || raw == nil {
		return
	}
	if list, ok := raw.([]string); ok {
		sort.Strings(list)
		container[key] = list
		return
	}
	if list, ok := raw.([]any); ok {
		out := make([]string, 0, len(list))
		for _, item := range list {
			str, ok := item.(string)
			if !ok {
				return
			}
			out = append(out, str)
		}
		sort.Strings(out)
		container[key] = out
	}
}
