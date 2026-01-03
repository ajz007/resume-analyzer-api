-- +goose Up
CREATE TABLE IF NOT EXISTS usage (
    user_id TEXT PRIMARY KEY,
    plan TEXT NOT NULL,
    limit_amount INT NOT NULL,
    used INT NOT NULL,
    resets_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS usage;
