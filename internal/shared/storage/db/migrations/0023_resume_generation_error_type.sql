-- +goose Up
ALTER TABLE resume_generation_jobs
    ADD COLUMN IF NOT EXISTS error_type TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_resume_generation_jobs_error_type ON resume_generation_jobs(error_type);

-- +goose Down
DROP INDEX IF EXISTS idx_resume_generation_jobs_error_type;
ALTER TABLE resume_generation_jobs
    DROP COLUMN IF EXISTS error_type;
