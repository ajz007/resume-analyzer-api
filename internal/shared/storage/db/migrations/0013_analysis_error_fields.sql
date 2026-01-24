-- +goose Up
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS error_code TEXT;
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS error_retryable BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE analyses DROP COLUMN IF EXISTS error_retryable;
ALTER TABLE analyses DROP COLUMN IF EXISTS error_code;
