-- +goose Up
ALTER TABLE analyses ADD COLUMN recheck_of UUID REFERENCES analyses(id);
CREATE INDEX idx_application_rechecks ON analyses (user_id, recheck_of, document_id, created_at DESC) WHERE recheck_of IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX idx_application_rechecks;
ALTER TABLE analyses DROP COLUMN recheck_of;
