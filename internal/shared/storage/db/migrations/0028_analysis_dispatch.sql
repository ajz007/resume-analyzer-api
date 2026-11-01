-- +goose Up
CREATE TABLE analysis_dispatch (
 analysis_id UUID PRIMARY KEY REFERENCES analyses(id),
 request_id TEXT NOT NULL DEFAULT '',
 attempts INT NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 claim_token UUID,
 claim_until TIMESTAMPTZ,
 published_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_analysis_dispatch_due ON analysis_dispatch(next_attempt_at);
-- Only established reservations can be safely adopted as charged queued work.
INSERT INTO analysis_dispatch(analysis_id,created_at)
SELECT a.id,a.created_at FROM analyses a JOIN analysis_usage_reservations r ON r.analysis_id=a.id
WHERE a.status='queued' AND r.state='reserved';
CREATE INDEX idx_analysis_recovery_queued ON analyses(created_at) WHERE status='queued';
CREATE INDEX idx_analysis_recovery_processing ON analyses(started_at) WHERE status='processing';

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
 RAISE EXCEPTION 'Dispatch history must be retained; roll back code without deleting recovery records.';
END;
$$;
-- +goose StatementEnd
