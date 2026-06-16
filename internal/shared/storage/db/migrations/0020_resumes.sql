-- +goose Up
CREATE TABLE IF NOT EXISTS resumes (
    id UUID PRIMARY KEY,
    owner_id TEXT NOT NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    current_version_id UUID,
    current_resume_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS resume_versions (
    id UUID PRIMARY KEY,
    resume_id UUID NOT NULL REFERENCES resumes(id),
    version_number INTEGER NOT NULL,
    source_type TEXT NOT NULL,
    resume_json JSONB NOT NULL,
    change_summary JSONB,
    parent_version_id UUID REFERENCES resume_versions(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE resumes
ADD CONSTRAINT resumes_current_version_id_fkey
FOREIGN KEY (current_version_id) REFERENCES resume_versions(id);

CREATE INDEX IF NOT EXISTS idx_resumes_owner_id ON resumes(owner_id);
CREATE INDEX IF NOT EXISTS idx_resume_versions_resume_id ON resume_versions(resume_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_resume_versions_resume_version_number ON resume_versions(resume_id, version_number);

-- +goose Down
DROP INDEX IF EXISTS idx_resume_versions_resume_version_number;
DROP INDEX IF EXISTS idx_resume_versions_resume_id;
DROP INDEX IF EXISTS idx_resumes_owner_id;

ALTER TABLE resumes DROP CONSTRAINT IF EXISTS resumes_current_version_id_fkey;

DROP TABLE IF EXISTS resume_versions;
DROP TABLE IF EXISTS resumes;
