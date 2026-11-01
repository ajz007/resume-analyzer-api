-- +goose Up
-- Display labels and a reusable-source preference do not mutate submitted evidence.
CREATE TABLE IF NOT EXISTS document_preferences (
 user_id TEXT PRIMARY KEY,
 document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS application_labels (
 analysis_id UUID PRIMARY KEY REFERENCES analyses(id) ON DELETE CASCADE,
 company_name TEXT NOT NULL DEFAULT '' CHECK (char_length(company_name)<=120),
 target_role TEXT NOT NULL DEFAULT '' CHECK (char_length(target_role)<=120),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS application_labels;
DROP TABLE IF EXISTS document_preferences;
