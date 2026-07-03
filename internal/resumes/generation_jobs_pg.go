package resumes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type GenerationJobPGRepo struct {
	DB *sql.DB
}

func (r *GenerationJobPGRepo) Create(ctx context.Context, job GenerationJob) error {
	const query = `
INSERT INTO resume_generation_jobs (
    id, owner_id, status, request_json, resume_id, version_id, result_json, error_type, error_message,
    created_at, updated_at, started_at, completed_at
) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13)`

	requestJSON, err := json.Marshal(job.Request)
	if err != nil {
		return err
	}
	resultJSON, err := marshalGenerationJobResult(job.Result)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(
		ctx,
		query,
		job.ID,
		job.OwnerID,
		job.Status,
		requestJSON,
		nullableString(job.ResumeID),
		nullableString(job.VersionID),
		resultJSON,
		job.ErrorType,
		job.ErrorMessage,
		job.CreatedAt,
		job.UpdatedAt,
		job.StartedAt,
		job.CompletedAt,
	)
	return err
}

func (r *GenerationJobPGRepo) GetByID(ctx context.Context, jobID string) (GenerationJob, error) {
	return r.getByID(ctx, r.DB, jobID)
}

func (r *GenerationJobPGRepo) Update(ctx context.Context, job GenerationJob) error {
	const query = `
UPDATE resume_generation_jobs
SET owner_id = $1,
    status = $2,
    request_json = $3::jsonb,
	    resume_id = $4,
	    version_id = $5,
	    result_json = $6::jsonb,
	    error_type = $7,
	    error_message = $8,
	    updated_at = $9,
	    started_at = $10,
	    completed_at = $11
	WHERE id = $12`

	requestJSON, err := json.Marshal(job.Request)
	if err != nil {
		return err
	}
	resultJSON, err := marshalGenerationJobResult(job.Result)
	if err != nil {
		return err
	}
	res, err := r.DB.ExecContext(
		ctx,
		query,
		job.OwnerID,
		job.Status,
		requestJSON,
		nullableString(job.ResumeID),
		nullableString(job.VersionID),
		resultJSON,
		job.ErrorType,
		job.ErrorMessage,
		job.UpdatedAt,
		job.StartedAt,
		job.CompletedAt,
		job.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GenerationJobPGRepo) ClaimQueued(ctx context.Context, jobID string, startedAt time.Time) (GenerationJob, bool, error) {
	const query = `
	UPDATE resume_generation_jobs
	SET status = 'processing',
	    started_at = $2,
	    updated_at = $2
	WHERE id = $1
	  AND status = 'queued'`

	res, err := r.DB.ExecContext(ctx, query, jobID, startedAt)
	if err != nil {
		return GenerationJob{}, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, err := r.GetByID(ctx, jobID)
		if err != nil {
			return GenerationJob{}, false, err
		}
		return job, false, nil
	}
	job, err := r.GetByID(ctx, jobID)
	return job, true, err
}

func (r *GenerationJobPGRepo) MarkCompleted(ctx context.Context, jobID, resumeID, versionID string, result *GenerationJobResult, completedAt time.Time) (GenerationJob, bool, error) {
	const query = `
	UPDATE resume_generation_jobs
		SET status = 'completed',
		    resume_id = $2,
		    version_id = $3,
		    result_json = $4::jsonb,
		    error_type = NULL,
		    error_message = NULL,
		    updated_at = $5,
		    completed_at = $5
	WHERE id = $1
	  AND status = 'processing'`

	resultJSON, err := marshalGenerationJobResult(result)
	if err != nil {
		return GenerationJob{}, false, err
	}
	res, err := r.DB.ExecContext(ctx, query, jobID, resumeID, versionID, resultJSON, completedAt)
	if err != nil {
		return GenerationJob{}, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, err := r.GetByID(ctx, jobID)
		if err != nil {
			return GenerationJob{}, false, err
		}
		return job, false, nil
	}
	job, err := r.GetByID(ctx, jobID)
	return job, true, err
}

func (r *GenerationJobPGRepo) MarkFailed(ctx context.Context, jobID, errorType, errorMessage string, completedAt time.Time) (GenerationJob, bool, error) {
	const query = `
		UPDATE resume_generation_jobs
		SET status = 'failed',
		    error_type = $2,
		    error_message = $3,
		    updated_at = $4,
		    completed_at = $4
		WHERE id = $1
		  AND status IN ('queued', 'processing')`

	res, err := r.DB.ExecContext(ctx, query, jobID, errorType, errorMessage, completedAt)
	if err != nil {
		return GenerationJob{}, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		job, err := r.GetByID(ctx, jobID)
		if err != nil {
			return GenerationJob{}, false, err
		}
		return job, false, nil
	}
	job, err := r.GetByID(ctx, jobID)
	return job, true, err
}

func marshalGenerationJobResult(result *GenerationJobResult) ([]byte, error) {
	if result == nil {
		return nil, nil
	}
	return json.Marshal(result)
}

const generationJobSelectByID = `
SELECT id, owner_id, status, request_json, resume_id, version_id, result_json, error_type, error_message,
       created_at, updated_at, started_at, completed_at
FROM resume_generation_jobs
WHERE id = $1
LIMIT 1`

type queryRowScanner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *GenerationJobPGRepo) getByID(ctx context.Context, q queryRowScanner, jobID string) (GenerationJob, error) {
	var job GenerationJob
	var requestJSON []byte
	var resultJSON []byte
	var resumeID sql.NullString
	var versionID sql.NullString
	var errorType sql.NullString
	var errorMessage sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime

	err := q.QueryRowContext(ctx, generationJobSelectByID, jobID).Scan(
		&job.ID,
		&job.OwnerID,
		&job.Status,
		&requestJSON,
		&resumeID,
		&versionID,
		&resultJSON,
		&errorType,
		&errorMessage,
		&job.CreatedAt,
		&job.UpdatedAt,
		&startedAt,
		&completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GenerationJob{}, ErrNotFound
		}
		return GenerationJob{}, err
	}
	if err := json.Unmarshal(requestJSON, &job.Request); err != nil {
		return GenerationJob{}, err
	}
	if resumeID.Valid {
		job.ResumeID = &resumeID.String
	}
	if versionID.Valid {
		job.VersionID = &versionID.String
	}
	if len(resultJSON) > 0 {
		var result GenerationJobResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			return GenerationJob{}, err
		}
		job.Result = &result
	}
	if errorType.Valid {
		job.ErrorType = &errorType.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = &errorMessage.String
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	return job, nil
}
