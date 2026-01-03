-- +goose Up
CREATE TABLE IF NOT EXISTS analyses (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL,
    user_id TEXT NOT NULL,
    status TEXT NOT NULL,
    result JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS analyses;
