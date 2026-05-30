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
	sharedcrypto "resume-backend/internal/shared/crypto"
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
	Repo             Repo
	Usage            *usage.Service
	DocRepo          documents.DocumentsRepo
	Store            object.ObjectStore
	LLM              llm.Client
	JobQueue         queue.Client
	Provider         string
	Model            string
	AnalysisVersion  string
	ShareTokenCipher *sharedcrypto.TokenCipher
	UIBaseURL        string
}

// Create enqueues a new analysis and kicks off asynchronous completion.
func (s *Service) Create(ctx context.Context, documentID, userID, jobDescription, promptVersion string) (Analysis, error) {
	if documentID == "" || userID == "" {
		return Analysis{}, errors.New("documentID and userID are required")
	}
	promptVersion = NormalizePromptVersion(promptVersion)

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
		Mode:            ModeJobMatch,
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
func (s *Service) StartOrReuse(ctx context.Context, documentID, userID, jobDescription, promptVersion string, mode AnalysisMode, allowRetry bool) (Analysis, bool, error) {
	if documentID == "" || userID == "" {
		return Analysis{}, false, errors.New("documentID and userID are required")
	}
	promptVersion = NormalizePromptVersion(promptVersion)
	if mode == "" {
		mode = ModeJobMatch
	}

	analysis := Analysis{
		ID:              uuid.NewString(),
		DocumentID:      documentID,
		UserID:          userID,
		JobDescription:  jobDescription,
		PromptVersion:   promptVersion,
		Mode:            mode,
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
	parseParser := "cache"
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
				if s.failParseAnalysisIfIssue(ctx, analysis, doc, err, &startedAt) {
					return err
				}
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			parseParser = parserNameForDocument(doc)
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
				if s.failParseAnalysisIfIssue(ctx, analysis, doc, err, &startedAt) {
					return err
				}
				s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
				return err
			}
			parseParser = parserNameForDocument(doc)
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
	if strings.TrimSpace(extracted) == "" {
		telemetry.Info("analysis.document.extracted_empty", map[string]any{
			"request_id":       requestID,
			"document_id":      doc.ID,
			"storage_provider": storageProvider,
		})
		extracted, extractedKey, err = s.reextractDocumentText(ctx, doc, storageProvider)
		if err != nil {
			err = fmt.Errorf("document %s mime %s: re-extract empty cached text: %w", doc.ID, doc.MimeType, err)
			if s.failParseAnalysisIfIssue(ctx, analysis, doc, err, &startedAt) {
				return err
			}
			s.failAnalysis(ctx, analysisID, analysis.UserID, analysis.DocumentID, err, &startedAt)
			return err
		}
		parseParser = parserNameForDocument(doc)
	}
	s.logParseTelemetry(ctx, analysis, doc, extract.ParseSuccess, parseParser, runeCount(extracted))

	input := llm.AnalyzeInput{
		ResumeText:     extracted,
		JobDescription: analysis.JobDescription,
		PromptVersion:  analysis.PromptVersion,
		TargetRole:     "",
	}
	var promptHash string
	ctxWithHash := llm.WithPromptHashCapture(ctx, &promptHash)
	ctxWithHash = withValidationTelemetryAnalysisID(ctxWithHash, analysisID)

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
	} else if analysis.PromptVersion == "v2_4" {
		raw, err = ValidateV2_4WithRetry(ctxWithHash, llmClient, input)
		if err != nil {
			err = fmt.Errorf("llm validate v2_4: %w", err)
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

func (s *Service) failParseAnalysisIfIssue(ctx context.Context, analysis Analysis, doc documents.Document, err error, startedAt *time.Time) bool {
	var issue *extract.ParseIssue
	if !errors.As(err, &issue) {
		return false
	}
	s.failParseAnalysis(ctx, analysis, doc, issue, startedAt)
	return true
}

func (s *Service) failParseAnalysis(ctx context.Context, analysis Analysis, doc documents.Document, issue *extract.ParseIssue, startedAt *time.Time) {
	if issue == nil {
		return
	}
	code := ErrorCodeUnsupportedFormat
	msg := issue.Message
	retryable := false
	result := parseIssueResult(issue)
	completedAt := time.Now().UTC()
	if updateErr := s.Repo.UpdateStatusResultAndError(context.Background(), analysis.ID, StatusFailed, result, &code, &msg, &retryable, nil, &completedAt); updateErr != nil {
		fmt.Printf("failParseAnalysis: update failed id=%s err=%v orig=%v\n", analysis.ID, updateErr, issue)
	}
	metrics.IncAnalysisFailed()
	if startedAt != nil {
		metrics.ObserveAnalysisDurationMs(durationMs(startedAt, &completedAt))
	}
	s.logParseTelemetry(ctx, analysis, doc, issue.Status, issue.Parser, issue.ExtractedCharCount)
	telemetry.Info("analysis.status", map[string]any{
		"request_id":        requestIDFromContext(ctx),
		"user_id":           analysis.UserID,
		"document_id":       analysis.DocumentID,
		"analysis_id":       analysis.ID,
		"status":            StatusFailed,
		"status_transition": "processing->failed",
		"duration_ms":       durationMs(startedAt, &completedAt),
		"error_code":        code,
	})
}

func parseIssueResult(issue *extract.ParseIssue) map[string]any {
	if issue == nil {
		return nil
	}
	status := issue.Status
	if status == "" {
		status = extract.ParseFailed
	}
	code := issue.Code
	if strings.TrimSpace(code) == "" {
		code = extract.ParseCodeUnsupportedResumeFormat
	}
	return map[string]any{
		"status":          string(status),
		"code":            code,
		"title":           fallbackString(issue.Title, "Unable to reliably read resume"),
		"message":         fallbackString(issue.Message, "Your resume appears to use formatting that may be difficult for ATS systems and resume parsers to read."),
		"recommendations": fallbackStringSlice(issue.Recommendations, extract.ParseRecommendations()),
		"atsInsight": map[string]any{
			"title":   fallbackString(issue.ATSInsightTitle, "Resume Format Warning"),
			"message": fallbackString(issue.ATSInsightMessage, "Your resume format may not be ATS-friendly. If our parser cannot reliably extract text, some ATS platforms may also struggle to process it."),
		},
	}
}

func isParseFailureAnalysis(analysis Analysis) bool {
	if analysis.Status != StatusFailed {
		return false
	}
	if analysis.ErrorCode != ErrorCodeUnsupportedFormat && analysis.ErrorCode != extract.ParseCodeUnsupportedResumeFormat {
		return false
	}
	if analysis.Result == nil {
		return false
	}
	status, _ := analysis.Result["status"].(string)
	return status == string(extract.ParseFailed) || status == string(extract.ParseLowConfidence)
}

func parseFailureResponse(analysis Analysis) map[string]any {
	resp := map[string]any{
		"analysisId": analysis.ID,
	}
	for _, key := range []string{"status", "code", "title", "message", "recommendations", "atsInsight"} {
		if value, ok := analysis.Result[key]; ok {
			resp[key] = value
		}
	}
	return resp
}

func fallbackStringSlice(value []string, fallback []string) []string {
	if len(value) == 0 {
		return fallback
	}
	return value
}

func (s *Service) logParseTelemetry(ctx context.Context, analysis Analysis, doc documents.Document, status extract.ParseStatus, parser string, charCount int) {
	event := "resume.parse_success"
	switch status {
	case extract.ParseFailed:
		event = "resume.parse_failed"
	case extract.ParseLowConfidence:
		event = "resume.parse_low_confidence"
	}
	telemetry.Info(event, map[string]any{
		"request_id":             requestIDFromContext(ctx),
		"user_id":                analysis.UserID,
		"document_id":            doc.ID,
		"analysis_id":            analysis.ID,
		"file_type":              doc.MimeType,
		"parser":                 fallbackString(parser, parserNameForDocument(doc)),
		"extracted_char_count":   charCount,
		"parse_status":           string(status),
		"storage_provider":       normalizeStorageProvider(doc.StorageProvider),
		"has_extracted_text_key": doc.ExtractedTextKey != "",
	})
}

func parserNameForDocument(doc documents.Document) string {
	mimeType := strings.ToLower(strings.TrimSpace(doc.MimeType))
	switch mimeType {
	case "application/pdf":
		return "github.com/ledongthuc/pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	default:
		return "unknown"
	}
}

func runeCount(value string) int {
	return len([]rune(strings.TrimSpace(value)))
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
	var parseIssue *extract.ParseIssue
	if errors.As(err, &parseIssue) {
		return ErrorCodeUnsupportedFormat, false
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

func (s *Service) reextractDocumentText(ctx context.Context, doc documents.Document, storageProvider string) (string, string, error) {
	extractedKey := doc.StorageKey + ".extracted.txt"
	switch storageProvider {
	case "s3":
		s3Client, err := newS3DocClient(ctx)
		if err != nil {
			return "", "", fmt.Errorf("s3 client: %w", err)
		}
		raw, err := s3Client.GetObjectBytes(ctx, doc.StorageKey)
		if err != nil {
			return "", "", fmt.Errorf("s3 read: %w", err)
		}
		extracted, err := extract.ExtractTextFromBytes(ctx, raw, doc.MimeType, doc.FileName)
		if err != nil {
			return "", "", err
		}
		if err := s3Client.PutText(ctx, extractedKey, extracted); err != nil {
			return "", "", fmt.Errorf("store extracted: %w", err)
		}
		if err := s.DocRepo.UpdateExtraction(ctx, doc.UserID, doc.ID, extractedKey, time.Now().UTC()); err != nil {
			return "", "", fmt.Errorf("update extraction: %w", err)
		}
		return extracted, extractedKey, nil
	default:
		extracted, err := extract.ExtractText(ctx, s.Store, doc.StorageKey, doc.MimeType, doc.FileName)
		if err != nil {
			return "", "", err
		}
		if err := s.DocRepo.UpdateExtraction(ctx, doc.UserID, doc.ID, extractedKey, time.Now().UTC()); err != nil {
			return "", "", fmt.Errorf("update extraction: %w", err)
		}
		return extracted, extractedKey, nil
	}
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
