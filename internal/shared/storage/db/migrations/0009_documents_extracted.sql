-- +goose Up
ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS extracted_text_key TEXT NULL,
    ADD COLUMN IF NOT EXISTS extracted_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE documents
    DROP COLUMN IF EXISTS extracted_text_key,
    DROP COLUMN IF EXISTS extracted_at;
