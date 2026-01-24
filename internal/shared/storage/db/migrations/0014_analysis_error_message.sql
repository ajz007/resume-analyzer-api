-- +goose Up
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS error_message TEXT;

-- +goose Down
ALTER TABLE analyses DROP COLUMN IF EXISTS error_message;
