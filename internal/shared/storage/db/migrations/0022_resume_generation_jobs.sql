-- +goose Up
CREATE TABLE IF NOT EXISTS resume_generation_jobs (
    id UUID PRIMARY KEY,
    owner_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    request_json JSONB NOT NULL,
    resume_id UUID NULL REFERENCES resumes(id),
    version_id UUID NULL REFERENCES resume_versions(id),
    result_json JSONB NULL,
    error_message TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_resume_generation_jobs_owner_id ON resume_generation_jobs(owner_id);
CREATE INDEX IF NOT EXISTS idx_resume_generation_jobs_status ON resume_generation_jobs(status);
CREATE INDEX IF NOT EXISTS idx_resume_generation_jobs_created_at ON resume_generation_jobs(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_resume_generation_jobs_created_at;
DROP INDEX IF EXISTS idx_resume_generation_jobs_status;
DROP INDEX IF EXISTS idx_resume_generation_jobs_owner_id;
DROP TABLE IF EXISTS resume_generation_jobs;
