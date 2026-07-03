package resumes

import (
	"context"
	"sync"
	"time"

	modelv1 "resume-backend/resume/modelv1"
)

type GenerationJobMemoryRepo struct {
	mu   sync.RWMutex
	byID map[string]GenerationJob
}

func NewGenerationJobMemoryRepo() *GenerationJobMemoryRepo {
	return &GenerationJobMemoryRepo{
		byID: make(map[string]GenerationJob),
	}
}

func (r *GenerationJobMemoryRepo) Create(ctx context.Context, job GenerationJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[job.ID]; exists {
		return ErrAlreadyExists
	}
	r.byID[job.ID] = cloneGenerationJob(job)
	return nil
}

func (r *GenerationJobMemoryRepo) GetByID(ctx context.Context, jobID string) (GenerationJob, error) {
	if err := ctx.Err(); err != nil {
		return GenerationJob{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.byID[jobID]
	if !ok {
		return GenerationJob{}, ErrNotFound
	}
	return cloneGenerationJob(job), nil
}

func (r *GenerationJobMemoryRepo) Update(ctx context.Context, job GenerationJob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[job.ID]; !ok {
		return ErrNotFound
	}
	r.byID[job.ID] = cloneGenerationJob(job)
	return nil
}

func (r *GenerationJobMemoryRepo) ClaimQueued(ctx context.Context, jobID string, startedAt time.Time) (GenerationJob, bool, error) {
	if err := ctx.Err(); err != nil {
		return GenerationJob{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[jobID]
	if !ok {
		return GenerationJob{}, false, ErrNotFound
	}
	if job.Status != GenerationJobStatusQueued {
		return cloneGenerationJob(job), false, nil
	}
	job.Status = GenerationJobStatusProcessing
	job.StartedAt = &startedAt
	job.UpdatedAt = startedAt
	r.byID[job.ID] = cloneGenerationJob(job)
	return cloneGenerationJob(job), true, nil
}

func (r *GenerationJobMemoryRepo) MarkCompleted(ctx context.Context, jobID, resumeID, versionID string, result *GenerationJobResult, completedAt time.Time) (GenerationJob, bool, error) {
	if err := ctx.Err(); err != nil {
		return GenerationJob{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[jobID]
	if !ok {
		return GenerationJob{}, false, ErrNotFound
	}
	if job.Status != GenerationJobStatusProcessing {
		return cloneGenerationJob(job), false, nil
	}
	job.Status = GenerationJobStatusCompleted
	job.UpdatedAt = completedAt
	job.CompletedAt = &completedAt
	job.ResumeID = stringPtr(resumeID)
	job.VersionID = stringPtr(versionID)
	job.ErrorType = nil
	job.ErrorMessage = nil
	if result != nil {
		cloned := *result
		cloned.RequiresUserInput = append([]modelv1.RequiresUserInput(nil), result.RequiresUserInput...)
		cloned.Assumptions = append([]modelv1.Assumption(nil), result.Assumptions...)
		cloned.Warnings = append([]modelv1.ResponseWarning(nil), result.Warnings...)
		job.Result = &cloned
	} else {
		job.Result = nil
	}
	r.byID[job.ID] = cloneGenerationJob(job)
	return cloneGenerationJob(job), true, nil
}

func (r *GenerationJobMemoryRepo) MarkFailed(ctx context.Context, jobID, errorType, errorMessage string, completedAt time.Time) (GenerationJob, bool, error) {
	if err := ctx.Err(); err != nil {
		return GenerationJob{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.byID[jobID]
	if !ok {
		return GenerationJob{}, false, ErrNotFound
	}
	if job.Status == GenerationJobStatusCompleted || job.Status == GenerationJobStatusFailed {
		return cloneGenerationJob(job), false, nil
	}
	job.Status = GenerationJobStatusFailed
	job.UpdatedAt = completedAt
	job.CompletedAt = &completedAt
	job.ErrorType = stringPtr(errorType)
	job.ErrorMessage = stringPtr(errorMessage)
	r.byID[job.ID] = cloneGenerationJob(job)
	return cloneGenerationJob(job), true, nil
}

func cloneGenerationJob(job GenerationJob) GenerationJob {
	out := job
	out.Request = job.Request
	if job.Result != nil {
		result := *job.Result
		result.RequiresUserInput = append([]modelv1.RequiresUserInput(nil), job.Result.RequiresUserInput...)
		result.Assumptions = append([]modelv1.Assumption(nil), job.Result.Assumptions...)
		result.Warnings = append([]modelv1.ResponseWarning(nil), job.Result.Warnings...)
		out.Result = &result
	}
	return out
}
