-- +goose Up
CREATE TABLE IF NOT EXISTS analysis_shares (
    id UUID PRIMARY KEY,
    analysis_id UUID NOT NULL REFERENCES analyses(id) ON DELETE CASCADE,
    owner_user_id TEXT NULL,
    owner_guest_id TEXT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_analysis_shares_token_hash
    ON analysis_shares(token_hash);

CREATE INDEX IF NOT EXISTS idx_analysis_shares_analysis_id
    ON analysis_shares(analysis_id);

-- +goose Down
DROP INDEX IF EXISTS idx_analysis_shares_analysis_id;
DROP INDEX IF EXISTS idx_analysis_shares_token_hash;
DROP TABLE IF EXISTS analysis_shares;
