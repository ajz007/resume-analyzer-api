-- +goose Up
-- A durable reservation precedes any provider order creation. One purchase per report.
CREATE TABLE payment_purchases (
    user_id TEXT NOT NULL,
    analysis_id UUID NOT NULL,
    payment_id UUID NOT NULL UNIQUE,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    legacy_ambiguous BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, analysis_id)
);
-- Preserve existing orders. Prefer paid access, otherwise the earliest payable order.
-- Historical duplicate orders remain intact and block further checkout pending support review.
INSERT INTO payment_purchases (user_id, analysis_id, payment_id, amount_minor, currency, created_at, legacy_ambiguous)
SELECT DISTINCT ON (user_id, analysis_id)
       user_id, analysis_id, id, amount_minor, currency, created_at,
       count(*) OVER (PARTITION BY user_id, analysis_id) > 1
FROM payments
ORDER BY user_id, analysis_id,
         CASE status WHEN 'paid' THEN 0 WHEN 'created' THEN 1 WHEN 'failed' THEN 2 ELSE 3 END,
         created_at, id;

-- +goose Down
-- Dropping these reservations could let uncertain provider orders be recreated.
-- Application rollback should leave this additive table in place.
-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION 'Purchase reservations must be retained. Roll back application code without dropping this table.';
END;
$$;
-- +goose StatementEnd
