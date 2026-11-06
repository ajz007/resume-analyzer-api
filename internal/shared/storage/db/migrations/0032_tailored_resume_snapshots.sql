-- +goose Up
ALTER TABLE documents ADD COLUMN tailored_analysis_id UUID REFERENCES analyses(id) ON DELETE SET NULL;
CREATE INDEX idx_documents_tailored_analysis ON documents (user_id, tailored_analysis_id, created_at DESC) WHERE tailored_analysis_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX idx_documents_tailored_analysis;
ALTER TABLE documents DROP COLUMN tailored_analysis_id;
