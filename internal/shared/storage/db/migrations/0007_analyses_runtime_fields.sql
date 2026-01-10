-- +goose Up
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE analyses ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_analyses_user_created_at ON analyses(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_analyses_status ON analyses(status);

-- +goose Down
DROP INDEX IF EXISTS idx_analyses_user_created_at;
DROP INDEX IF EXISTS idx_analyses_status;

ALTER TABLE analyses DROP COLUMN IF EXISTS error_message;
ALTER TABLE analyses DROP COLUMN IF EXISTS started_at;
ALTER TABLE analyses DROP COLUMN IF EXISTS completed_at;
ALTER TABLE analyses DROP COLUMN IF EXISTS updated_at;
