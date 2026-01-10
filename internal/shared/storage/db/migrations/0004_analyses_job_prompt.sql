-- +goose Up
ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS job_description TEXT,
    ADD COLUMN IF NOT EXISTS prompt_version TEXT;

-- +goose Down
ALTER TABLE analyses
    DROP COLUMN IF EXISTS job_description,
    DROP COLUMN IF EXISTS prompt_version;
