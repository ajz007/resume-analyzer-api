-- +goose Up
ALTER TABLE analyses
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'openai',
    ADD COLUMN IF NOT EXISTS model TEXT NOT NULL DEFAULT '',
    ALTER COLUMN job_description SET DEFAULT '',
    ALTER COLUMN prompt_version SET DEFAULT 'v1';

UPDATE analyses
SET job_description = ''
WHERE job_description IS NULL;

UPDATE analyses
SET prompt_version = 'v1'
WHERE prompt_version IS NULL;

ALTER TABLE analyses
    ALTER COLUMN job_description SET NOT NULL,
    ALTER COLUMN prompt_version SET NOT NULL;

-- +goose Down
ALTER TABLE analyses
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS model,
    ALTER COLUMN job_description DROP NOT NULL,
    ALTER COLUMN prompt_version DROP NOT NULL,
    ALTER COLUMN job_description DROP DEFAULT,
    ALTER COLUMN prompt_version DROP DEFAULT;
