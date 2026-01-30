-- +goose Up
ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS storage_provider TEXT NOT NULL DEFAULT 'local',
    ADD COLUMN IF NOT EXISTS original_filename TEXT NULL,
    ADD COLUMN IF NOT EXISTS content_type TEXT NULL;

ALTER TABLE documents
    ALTER COLUMN storage_key DROP NOT NULL,
    ALTER COLUMN size_bytes DROP NOT NULL;

UPDATE documents
SET storage_provider = 'local'
WHERE storage_provider IS NULL OR storage_provider = '';

UPDATE documents
SET original_filename = file_name
WHERE original_filename IS NULL;

UPDATE documents
SET content_type = mime_type
WHERE content_type IS NULL;

-- +goose Down
ALTER TABLE documents
    ALTER COLUMN storage_key SET NOT NULL,
    ALTER COLUMN size_bytes SET NOT NULL;

ALTER TABLE documents
    DROP COLUMN IF EXISTS content_type,
    DROP COLUMN IF EXISTS original_filename,
    DROP COLUMN IF EXISTS storage_provider;
