-- +goose Up
ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'JOB_MATCH';

UPDATE analyses
SET mode = 'JOB_MATCH'
WHERE mode IS NULL OR mode = '';

-- +goose Down
ALTER TABLE analyses
    DROP COLUMN IF EXISTS mode;
