-- +goose Up
CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL,
  full_name TEXT,
  given_name TEXT,
  family_name TEXT,
  picture_url TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS given_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS family_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS picture_url TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;

UPDATE users SET updated_at = now() WHERE updated_at IS NULL;
ALTER TABLE users ALTER COLUMN updated_at SET DEFAULT now();

-- +goose Down
-- Intentionally no-op to avoid destructive user data loss.
