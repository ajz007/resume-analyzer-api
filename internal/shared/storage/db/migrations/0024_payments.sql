-- +goose Up
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    analysis_id UUID NOT NULL,
    razorpay_order_id TEXT NOT NULL UNIQUE,
    razorpay_payment_id TEXT NULL,
    amount_minor BIGINT NOT NULL,
    currency TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('created', 'paid', 'failed', 'refunded')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_payments_user_analysis ON payments(user_id, analysis_id);
CREATE INDEX IF NOT EXISTS idx_payments_razorpay_payment_id ON payments(razorpay_payment_id);

CREATE TABLE IF NOT EXISTS report_unlocks (
    user_id TEXT NOT NULL,
    analysis_id UUID NOT NULL,
    payment_id UUID NOT NULL REFERENCES payments(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, analysis_id)
);

-- +goose Down
DROP TABLE IF EXISTS report_unlocks;
DROP INDEX IF EXISTS idx_payments_razorpay_payment_id;
DROP INDEX IF EXISTS idx_payments_user_analysis;
DROP TABLE IF EXISTS payments;
