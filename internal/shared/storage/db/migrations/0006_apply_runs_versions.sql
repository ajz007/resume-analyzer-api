-- +goose Up
CREATE TABLE IF NOT EXISTS apply_runs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    analysis_id TEXT NOT NULL,
    status TEXT NOT NULL,
    auto_fixes_count INT NOT NULL DEFAULT 0,
    safe_rewrites_count INT NOT NULL DEFAULT 0,
    blocked_rewrites_count INT NOT NULL DEFAULT 0,
    needs_input_count INT NOT NULL DEFAULT 0,
    placeholders_remaining INT NOT NULL DEFAULT 0,
    document_version_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS apply_runs_user_id_idx ON apply_runs(user_id);
CREATE INDEX IF NOT EXISTS apply_runs_analysis_id_idx ON apply_runs(analysis_id);

CREATE TABLE IF NOT EXISTS document_versions (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    apply_run_id TEXT,
    file_name TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS document_versions_document_id_idx ON document_versions(document_id);
CREATE INDEX IF NOT EXISTS document_versions_apply_run_id_idx ON document_versions(apply_run_id);

-- +goose Down
DROP TABLE IF EXISTS document_versions;
DROP TABLE IF EXISTS apply_runs;
