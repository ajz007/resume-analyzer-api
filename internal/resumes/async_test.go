package resumes

import (
	"context"
	"errors"
	"testing"
	"time"

	"resume-backend/internal/queue"
	modelv1 "resume-backend/resume/modelv1"
)

type failingQueueClient struct {
	err error
}

func (q failingQueueClient) Send(ctx context.Context, msg queue.Message) error {
	_ = ctx
	_ = msg
	if q.err != nil {
		return q.err
	}
	return errors.New("queue send failed")
}

func TestEnqueueGenerationMarksJobFailedWhenQueueSendFails(t *testing.T) {
	jobRepo := NewGenerationJobMemoryRepo()
	svc := &Service{
		JobRepo:  jobRepo,
		JobQueue: failingQueueClient{err: errors.New("sqs unavailable")},
	}

	_, err := svc.EnqueueGeneration(context.Background(), "google:12345", GenerateRequest{
		Title:          "Backend Engineer Resume",
		ExperienceText: "Built Go APIs.",
	})
	if err == nil {
		t.Fatal("expected queue send failure")
	}
	if len(jobRepo.byID) != 1 {
		t.Fatalf("expected one job, got %d", len(jobRepo.byID))
	}
	for _, job := range jobRepo.byID {
		if job.Status != GenerationJobStatusFailed {
			t.Fatalf("expected failed status, got %#v", job)
		}
		if job.ErrorType == nil || *job.ErrorType != "internal" {
			t.Fatalf("expected internal error type, got %#v", job.ErrorType)
		}
		if job.ErrorMessage == nil || *job.ErrorMessage != "internal: sqs unavailable" {
			t.Fatalf("expected sanitized error message, got %#v", job.ErrorMessage)
		}
	}
}

func TestFailGenerationJobStoresInternalReasonButPublicMessageStaysGeneric(t *testing.T) {
	jobRepo := NewGenerationJobMemoryRepo()
	svc := &Service{JobRepo: jobRepo}
	now := time.Now().UTC()
	job := GenerationJob{
		ID:        "job-timeout",
		OwnerID:   "google:12345",
		Status:    GenerationJobStatusProcessing,
		Request:   GenerateRequest{Title: "Backend Engineer Resume", ExperienceText: "Built Go APIs."},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	}
	if err := jobRepo.Create(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	err := svc.failGenerationJob(context.Background(), job, ErrGenerationTimeout)
	if !errors.Is(err, ErrGenerationTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}

	updated, getErr := jobRepo.GetByID(context.Background(), job.ID)
	if getErr != nil {
		t.Fatalf("get updated job: %v", getErr)
	}
	if updated.ErrorMessage == nil || *updated.ErrorMessage != "timeout: resume generation timed out" {
		t.Fatalf("expected internal timeout reason, got %#v", updated.ErrorMessage)
	}
	if updated.ErrorType == nil || *updated.ErrorType != "timeout" {
		t.Fatalf("expected timeout error type, got %#v", updated.ErrorType)
	}
}

func TestProcessResumeGenerationJobCompletesRecoveredProcessingJob(t *testing.T) {
	repo := NewMemoryRepo()
	jobRepo := NewGenerationJobMemoryRepo()
	svc := &Service{
		Repo:    repo,
		JobRepo: jobRepo,
	}
	jobID := "job-123"
	now := time.Now().UTC()
	job := GenerationJob{
		ID:        jobID,
		OwnerID:   "google:12345",
		Status:    GenerationJobStatusProcessing,
		Request:   GenerateRequest{Title: "Backend Engineer Resume", ExperienceText: "Built Go APIs."},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: &now,
	}
	if err := jobRepo.Create(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	expectedResumeID := deterministicGenerationObjectID(jobID, "resume")
	expectedVersionID := deterministicGenerationObjectID(jobID, "version")
	created, err := svc.createWithOptions(context.Background(), job.OwnerID, job.Request.Title, structurallyValidResumeForAsyncTest(), createResumeOptions{
		SourceType: SourceAIGenerated,
		OriginType: OriginAIGenerated,
		ResumeID:   expectedResumeID,
		VersionID:  expectedVersionID,
	})
	if err != nil {
		t.Fatalf("seed deterministic resume: %v", err)
	}

	if err := svc.ProcessResumeGenerationJob(context.Background(), jobID); err != nil {
		t.Fatalf("process generation job: %v", err)
	}
	updated, err := jobRepo.GetByID(context.Background(), jobID)
	if err != nil {
		t.Fatalf("get updated job: %v", err)
	}
	if updated.Status != GenerationJobStatusCompleted {
		t.Fatalf("expected completed job, got %#v", updated)
	}
	if updated.ResumeID == nil || *updated.ResumeID != created.Resume.ID {
		t.Fatalf("expected resume id %q, got %#v", created.Resume.ID, updated.ResumeID)
	}
	if updated.VersionID == nil || *updated.VersionID != created.Resume.CurrentVersionID {
		t.Fatalf("expected version id %q, got %#v", created.Resume.CurrentVersionID, updated.VersionID)
	}
}

func structurallyValidResumeForAsyncTest() modelv1.ResumeModel {
	return modelv1.ResumeModel{
		SchemaVersion: modelv1.SchemaVersion,
		SectionOrder:  []string{},
	}
}
