-- +goose Up
-- Shared operator policy: no per-Lambda configuration drift or new AWS resource.
CREATE TABLE ai_spend_policy (
 id BOOLEAN PRIMARY KEY DEFAULT true CHECK(id),
 calls_enabled BOOLEAN NOT NULL DEFAULT true,
 daily_limit_nano_usd BIGINT NOT NULL CHECK(daily_limit_nano_usd BETWEEN 0 AND 1000000000000000),
 max_request_bytes INT NOT NULL DEFAULT 262144 CHECK(max_request_bytes BETWEEN 1024 AND 1048576),
 max_completion_tokens INT NOT NULL DEFAULT 32768 CHECK(max_completion_tokens BETWEEN 1 AND 128000),
 max_attempts_per_operation INT NOT NULL DEFAULT 4 CHECK(max_attempts_per_operation BETWEEN 1 AND 8)
);
-- Owner selected USD50/day. Nano-USD avoids floating-point budget arithmetic.
INSERT INTO ai_spend_policy(id,daily_limit_nano_usd) VALUES(true,50000000000);
CREATE TABLE ai_spend_days (
 day_utc DATE PRIMARY KEY,
 booked_nano_usd BIGINT NOT NULL DEFAULT 0 CHECK(booked_nano_usd>=0),
 legacy_uncertain_cover_nano_usd BIGINT CHECK(legacy_uncertain_cover_nano_usd>=0),
 breach_detected BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE ai_spend_operations (
 operation_id UUID PRIMARY KEY,
 admitted_attempts INT NOT NULL DEFAULT 0
);
CREATE TABLE ai_spend_holds (
 attempt_id UUID PRIMARY KEY REFERENCES ai_usage_attempts(id),
 operation_id UUID NOT NULL REFERENCES ai_spend_operations(operation_id),
 day_utc DATE NOT NULL REFERENCES ai_spend_days(day_utc),
 reserved_nano_usd BIGINT NOT NULL CHECK(reserved_nano_usd>0),
 booked_nano_usd BIGINT NOT NULL CHECK(booked_nano_usd>=0),
 state TEXT NOT NULL DEFAULT 'held' CHECK(state IN ('held','settled')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_spend_unsettled ON ai_spend_holds(day_utc) WHERE state='held';
CREATE INDEX idx_ai_spend_operation ON ai_spend_holds(operation_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Retain spend holds and policy on rollback; do not forget uncertain provider spend.'; END; $$;
-- +goose StatementEnd
