-- +goose Up
ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS analysis_raw JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS analysis_result JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS analysis_completed_at TIMESTAMPTZ NULL;

-- +goose Down
ALTER TABLE analyses DROP COLUMN IF EXISTS analysis_completed_at;
ALTER TABLE analyses DROP COLUMN IF EXISTS analysis_result;
ALTER TABLE analyses DROP COLUMN IF EXISTS analysis_raw;
