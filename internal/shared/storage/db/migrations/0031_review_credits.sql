-- +goose Up
CREATE TABLE review_credit_orders (
 id UUID PRIMARY KEY,
 user_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_review_credit_orders_owner ON review_credit_orders(user_id, created_at);
CREATE TABLE review_credit_uses (
 analysis_id UUID NOT NULL REFERENCES analyses(id),
 payment_id UUID NOT NULL REFERENCES payments(id),
 user_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 released_at TIMESTAMPTZ,
 PRIMARY KEY (analysis_id, payment_id)
);
CREATE UNIQUE INDEX idx_review_credit_active_review ON review_credit_uses(analysis_id) WHERE released_at IS NULL;
CREATE UNIQUE INDEX idx_review_credit_active_use ON review_credit_uses(payment_id) WHERE released_at IS NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Keep purchased credit and usage history during rollback'; END; $$;
-- +goose StatementEnd
