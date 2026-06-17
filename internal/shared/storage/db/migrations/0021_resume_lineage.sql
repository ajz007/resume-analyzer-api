-- +goose Up
ALTER TABLE resumes
ADD COLUMN IF NOT EXISTS source_resume_id UUID NULL;

ALTER TABLE resumes
ADD COLUMN IF NOT EXISTS source_version_id UUID NULL;

ALTER TABLE resumes
ADD COLUMN IF NOT EXISTS origin_type TEXT NULL;

ALTER TABLE resume_versions
ADD COLUMN IF NOT EXISTS source_version_id UUID NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'resumes_source_resume_id_fkey'
          AND conrelid = 'resumes'::regclass
    ) THEN
        ALTER TABLE resumes
        ADD CONSTRAINT resumes_source_resume_id_fkey
        FOREIGN KEY (source_resume_id) REFERENCES resumes(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'resumes_source_version_id_fkey'
          AND conrelid = 'resumes'::regclass
    ) THEN
        ALTER TABLE resumes
        ADD CONSTRAINT resumes_source_version_id_fkey
        FOREIGN KEY (source_version_id) REFERENCES resume_versions(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'resume_versions_source_version_id_fkey'
          AND conrelid = 'resume_versions'::regclass
    ) THEN
        ALTER TABLE resume_versions
        ADD CONSTRAINT resume_versions_source_version_id_fkey
        FOREIGN KEY (source_version_id) REFERENCES resume_versions(id);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'resumes_origin_type_check'
          AND conrelid = 'resumes'::regclass
    ) THEN
        ALTER TABLE resumes
        ADD CONSTRAINT resumes_origin_type_check
        CHECK (origin_type IN (
            'blank',
            'manual',
            'parsed_from_upload',
            'ai_generated',
            'ai_tailored'
        ));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_resumes_source_resume_id ON resumes(source_resume_id);
CREATE INDEX IF NOT EXISTS idx_resumes_source_version_id ON resumes(source_version_id);
CREATE INDEX IF NOT EXISTS idx_resumes_origin_type ON resumes(origin_type);
CREATE INDEX IF NOT EXISTS idx_resume_versions_source_version_id ON resume_versions(source_version_id);

-- +goose Down
DROP INDEX IF EXISTS idx_resume_versions_source_version_id;
DROP INDEX IF EXISTS idx_resumes_origin_type;
DROP INDEX IF EXISTS idx_resumes_source_version_id;
DROP INDEX IF EXISTS idx_resumes_source_resume_id;

ALTER TABLE resume_versions DROP CONSTRAINT IF EXISTS resume_versions_source_version_id_fkey;
ALTER TABLE resumes DROP CONSTRAINT IF EXISTS resumes_origin_type_check;
ALTER TABLE resumes DROP CONSTRAINT IF EXISTS resumes_source_version_id_fkey;
ALTER TABLE resumes DROP CONSTRAINT IF EXISTS resumes_source_resume_id_fkey;

ALTER TABLE resume_versions DROP COLUMN IF EXISTS source_version_id;
ALTER TABLE resumes DROP COLUMN IF EXISTS origin_type;
ALTER TABLE resumes DROP COLUMN IF EXISTS source_version_id;
ALTER TABLE resumes DROP COLUMN IF EXISTS source_resume_id;
