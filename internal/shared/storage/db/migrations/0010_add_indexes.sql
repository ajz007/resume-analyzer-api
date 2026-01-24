-- +goose Up

-- 1) FK integrity (safe rollout)
ALTER TABLE analyses
  ADD CONSTRAINT analyses_document_id_fkey
  FOREIGN KEY (document_id) REFERENCES documents(id)
  NOT VALID;

-- 2) Constrain status values (safe rollout)
ALTER TABLE analyses
  ADD CONSTRAINT analyses_status_check
  CHECK (status IN ('queued','processing','completed','failed'))
  NOT VALID;

-- 3) Index for per-user latest/list queries
-- Use the partial index version ONLY if queries filter deleted_at IS NULL
CREATE INDEX IF NOT EXISTS idx_documents_user_created_at
  ON documents (user_id, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_documents_user_created_at;

ALTER TABLE analyses DROP CONSTRAINT IF EXISTS analyses_status_check;

ALTER TABLE analyses DROP CONSTRAINT IF EXISTS analyses_document_id_fkey;
