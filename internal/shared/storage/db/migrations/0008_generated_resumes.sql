-- +goose Up
CREATE TABLE IF NOT EXISTS generated_resumes (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    document_id UUID NOT NULL,
    analysis_id UUID NOT NULL,
    template_id TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_generated_resumes_user_created_at ON generated_resumes(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_generated_resumes_analysis_id ON generated_resumes(analysis_id);

-- +goose Down
DROP INDEX IF EXISTS idx_generated_resumes_user_created_at;
DROP INDEX IF EXISTS idx_generated_resumes_analysis_id;

DROP TABLE IF EXISTS generated_resumes;
