-- +goose Up
ALTER TABLE usage ADD COLUMN usage_epoch BIGINT NOT NULL DEFAULT 0;
CREATE TABLE analysis_usage_reservations (
    analysis_id UUID PRIMARY KEY REFERENCES analyses(id),
    user_id TEXT NOT NULL,
    resets_at TIMESTAMPTZ NOT NULL,
    usage_epoch BIGINT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('reserved','committed','released')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at TIMESTAMPTZ
);
CREATE INDEX idx_analysis_usage_unsettled ON analysis_usage_reservations(user_id) WHERE state='reserved';
-- No inferred backfill: legacy counters cannot prove which analyses were charged.

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'Review usage reservations must be retained; roll back code without deleting usage history.';
END;
$$;
-- +goose StatementEnd
