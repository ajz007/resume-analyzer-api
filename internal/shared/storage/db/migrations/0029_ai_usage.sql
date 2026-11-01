-- +goose Up
CREATE TABLE ai_usage_attempts (
 id UUID PRIMARY KEY,
 operation_id UUID NOT NULL,
 operation TEXT NOT NULL,
 analysis_id UUID,
 object_id UUID,
 stage TEXT NOT NULL,
 retry_kind TEXT NOT NULL DEFAULT '',
 attempt_number BIGINT NOT NULL,
 provider TEXT NOT NULL,
 requested_model TEXT NOT NULL,
 response_model TEXT NOT NULL DEFAULT '',
 provider_request_id TEXT NOT NULL DEFAULT '',
 response_id TEXT NOT NULL DEFAULT '',
 service_tier TEXT NOT NULL DEFAULT '',
 started_at TIMESTAMPTZ NOT NULL,
 finished_at TIMESTAMPTZ,
 latency_ms BIGINT NOT NULL DEFAULT 0,
 http_status INT NOT NULL DEFAULT 0,
 outcome TEXT NOT NULL,
 usage_state TEXT NOT NULL CHECK(usage_state IN ('pending','known','unknown')),
 unknown_reason TEXT NOT NULL DEFAULT '',
 input_tokens BIGINT,
 output_tokens BIGINT,
 total_tokens BIGINT,
 cached_tokens BIGINT,
 cache_write_tokens BIGINT,
 reasoning_tokens BIGINT,
 estimate_nano_usd BIGINT,
 pricing_version TEXT NOT NULL DEFAULT '',
 unpriced_reason TEXT NOT NULL DEFAULT '',
 UNIQUE(operation_id,attempt_number)
);
CREATE INDEX idx_ai_usage_started ON ai_usage_attempts(started_at);
CREATE INDEX idx_ai_usage_analysis ON ai_usage_attempts(analysis_id) WHERE analysis_id IS NOT NULL;
-- No prompt, answer, resume, request body, raw provider error or API key is stored.
-- No foreign keys to product objects: retained usage is accounting metadata only.


-- Internal read-only reporting views. No public endpoint is added.
CREATE VIEW ai_usage_daily AS
SELECT (started_at AT TIME ZONE 'UTC')::date AS day_utc, operation, requested_model, response_model, service_tier,
 count(*) AS recorded_attempts,
 count(*) FILTER (WHERE usage_state<>'known') AS pending_or_unknown_attempts,
 count(*) FILTER (WHERE usage_state='known' AND estimate_nano_usd IS NULL) AS known_unpriced_attempts,
 sum(input_tokens) AS observed_input_tokens, sum(output_tokens) AS observed_output_tokens,
 sum(cached_tokens) AS observed_cached_tokens, sum(cache_write_tokens) AS observed_cache_write_tokens,
 sum(reasoning_tokens) AS observed_reasoning_tokens,
 sum(estimate_nano_usd)::numeric / 1000000000 AS observed_priced_subtotal_usd,
 CASE WHEN count(estimate_nano_usd)=count(*) THEN sum(estimate_nano_usd)::numeric / 1000000000 END AS recorded_estimate_usd
FROM ai_usage_attempts GROUP BY 1,2,3,4,5;

CREATE VIEW ai_analysis_costs AS
WITH totals AS (
 SELECT analysis_id,count(*) AS recorded_attempts,
 count(*) FILTER (WHERE usage_state<>'known') AS pending_or_unknown_attempts,
 count(*) FILTER (WHERE usage_state='known' AND estimate_nano_usd IS NULL) AS known_unpriced_attempts,
 sum(estimate_nano_usd)::numeric / 1000000000 AS observed_priced_subtotal_usd,
 CASE WHEN count(estimate_nano_usd)=count(*) THEN sum(estimate_nano_usd)::numeric / 1000000000 END AS recorded_estimate_usd
 FROM ai_usage_attempts WHERE analysis_id IS NOT NULL GROUP BY analysis_id
)
SELECT a.id AS analysis_id,a.status,a.created_at,(a.deleted_at IS NOT NULL) AS deleted,coalesce(t.recorded_attempts,0) AS recorded_attempts,
 t.pending_or_unknown_attempts,t.known_unpriced_attempts,t.observed_priced_subtotal_usd,t.recorded_estimate_usd
FROM analyses a LEFT JOIN totals t ON t.analysis_id=a.id;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Retain provider usage history on application rollback.'; END; $$;
-- +goose StatementEnd
