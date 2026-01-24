-- +goose Up
ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS analysis_version TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS prompt_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE analyses DROP COLUMN IF EXISTS prompt_hash;
ALTER TABLE analyses DROP COLUMN IF EXISTS analysis_version;
