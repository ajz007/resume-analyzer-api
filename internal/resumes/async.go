package resumes

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"resume-backend/internal/queue"
	"resume-backend/internal/shared/telemetry"
	modelv1 "resume-backend/resume/modelv1"
)

const generationFailedMessage = "Resume generation failed. Please try again."

func (s *Service) EnqueueGeneration(ctx context.Context, ownerID string, req GenerateRequest) (GenerationJob, error) {
	ownerID = strings.TrimSpace(ownerID)
	req = normalizeGenerateRequest(req)
	if ownerID == "" {
		return GenerationJob{}, ErrInvalidInput
	}
	if s.JobRepo == nil || s.JobQueue == nil {
		return GenerationJob{}, ErrJobQueueNotConfigured
	}
	now := time.Now().UTC()
	job := GenerationJob{
		ID:        uuid.NewString(),
		OwnerID:   ownerID,
		Status:    GenerationJobStatusQueued,
		Request:   req,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.JobRepo.Create(ctx, job); err != nil {
		return GenerationJob{}, err
	}
	if err := s.JobQueue.Send(ctx, queueMessageForResumeGeneration(job.ID, requestIDFromContext(ctx))); err != nil {
		failedAt := time.Now().UTC()
		errorType := classifyGenerationErrorType(err)
		internalMessage := internalGenerationErrorMessage(err)
		_, _, markErr := s.JobRepo.MarkFailed(context.Background(), job.ID, errorType, internalMessage, failedAt)
		telemetry.Error("resume.generate.enqueue.failed", map[string]any{
			"request_id":    requestIDFromContext(ctx),
			"user_id":       ownerID,
			"generation_id": job.ID,
			"error_type":    errorType,
			"error":         sanitizeGenerationError(err),
		})
		if markErr != nil {
			return GenerationJob{}, markErr
		}
		return GenerationJob{}, err
	}
	telemetry.Info("resume.generate.accepted", map[string]any{
		"request_id":             requestIDFromContext(ctx),
		"user_id":                ownerID,
		"generation_id":          job.ID,
		"generation_mode":        req.GenerationMode,
		"job_description_length": len([]rune(req.JobDescription)),
		"experience_text_length": len([]rune(req.ExperienceText)),
	})
	return job, nil
}

func sanitizeGenerationError(err error) string {
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

func (s *Service) GetGenerationJob(ctx context.Context, ownerID, generationID string) (GenerationJob, error) {
	if s.JobRepo == nil {
		return GenerationJob{}, ErrNotFound
	}
	job, err := s.JobRepo.GetByID(ctx, strings.TrimSpace(generationID))
	if err != nil {
		return GenerationJob{}, err
	}
	if strings.TrimSpace(ownerID) == "" {
		return GenerationJob{}, ErrInvalidInput
	}
	if job.OwnerID != strings.TrimSpace(ownerID) {
		return GenerationJob{}, ErrForbidden
	}
	return job, nil
}

func (s *Service) ProcessResumeGenerationJob(ctx context.Context, generationID string) error {
	if s.JobRepo == nil {
		return ErrNotFound
	}
	jobID := strings.TrimSpace(generationID)
	job, err := s.JobRepo.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case GenerationJobStatusCompleted, GenerationJobStatusFailed:
		return nil
	case GenerationJobStatusProcessing:
		return s.completeRecoveredProcessingJob(ctx, job)
	}

	startedAt := time.Now().UTC()
	job, claimed, err := s.JobRepo.ClaimQueued(ctx, jobID, startedAt)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	requestID := requestIDFromContext(ctx)
	ctx = withRequestLogContext(ctx, requestID, job.OwnerID)
	ctx = withGenerationJobID(ctx, job.ID)
	telemetry.Info("resume.generate.worker.started", map[string]any{
		"request_id":    requestID,
		"user_id":       job.OwnerID,
		"generation_id": job.ID,
	})

	result, err := s.Generate(ctx, job.OwnerID, job.Request)
	if err != nil {
		return s.failGenerationJob(ctx, job, err)
	}

	completedAt := time.Now().UTC()
	finalResult := &GenerationJobResult{
		RequiresUserInput: append([]modelv1.RequiresUserInput(nil), result.RequiresUserInput...),
		Assumptions:       append([]modelv1.Assumption(nil), result.Assumptions...),
		Warnings:          append([]modelv1.ResponseWarning(nil), result.Warnings...),
		GenerationMode:    result.GenerationMode,
		FallbackUsed:      result.FallbackUsed,
		FallbackReason:    result.FallbackReason,
		DraftType:         result.DraftType,
	}
	if _, updated, err := s.JobRepo.MarkCompleted(ctx, job.ID, result.SavedResume.ID, result.SavedResume.CurrentVersionID, finalResult, completedAt); err != nil {
		return err
	} else if !updated {
		return nil
	}
	telemetry.Info("resume.generate.worker.saved", map[string]any{
		"request_id":    requestID,
		"user_id":       job.OwnerID,
		"generation_id": job.ID,
		"resume_id":     result.SavedResume.ID,
		"version_id":    result.SavedResume.CurrentVersionID,
	})
	return nil
}

func (s *Service) failGenerationJob(ctx context.Context, job GenerationJob, err error) error {
	now := time.Now().UTC()
	errorType := classifyGenerationErrorType(err)
	internalMessage := internalGenerationErrorMessage(err)
	if _, _, updateErr := s.JobRepo.MarkFailed(context.Background(), job.ID, errorType, internalMessage, now); updateErr != nil {
		return updateErr
	}
	telemetry.Error("resume.generate.worker.failed", map[string]any{
		"request_id":    requestIDFromContext(ctx),
		"user_id":       job.OwnerID,
		"generation_id": job.ID,
		"error_type":    errorType,
		"error":         sanitizeGenerationError(err),
	})
	return err
}

func internalGenerationErrorMessage(err error) string {
	switch {
	case err == nil:
		return generationFailedMessage
	case err == ErrGenerationTimeout:
		return "timeout: resume generation timed out"
	case err == ErrInvalidLLMOutput:
		return "invalid_output: model output failed validation"
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input: generation request was invalid"
	default:
		msg := sanitizeGenerationError(err)
		if msg == "" {
			return generationFailedMessage
		}
		return classifyGenerationErrorType(err) + ": " + msg
	}
}

func (s *Service) completeRecoveredProcessingJob(ctx context.Context, job GenerationJob) error {
	resumeID := deterministicGenerationObjectID(job.ID, "resume")
	resume, err := s.Repo.GetByID(ctx, job.OwnerID, resumeID)
	if err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	completedAt := time.Now().UTC()
	_, _, err = s.JobRepo.MarkCompleted(ctx, job.ID, resume.ID, resume.CurrentVersionID, job.Result, completedAt)
	return err
}

func queueMessageForResumeGeneration(jobID, requestID string) queue.Message {
	return queue.Message{
		Type:       queue.MessageTypeResumeGeneration,
		JobID:      jobID,
		RequestID:  requestID,
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Version:    1,
	}
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func classifyGenerationErrorType(err error) string {
	switch {
	case err == nil:
		return ""
	case err == ErrGenerationTimeout:
		return "timeout"
	case err == ErrInvalidLLMOutput:
		return "invalid_output"
	case err == ErrInvalidInput:
		return "invalid_input"
	default:
		return "internal"
	}
}
