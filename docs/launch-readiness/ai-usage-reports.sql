-- Run manually in the existing database SQL editor, with an appropriate rollout/cohort date.
-- Estimates use the recorded reference rate version; they are not provider invoices.

-- Coverage and reference-priced subtotals by UTC day and operation.
SELECT * FROM ai_usage_daily
WHERE day_utc >= (now() AT TIME ZONE 'UTC')::date - 30
ORDER BY day_utc DESC,operation,requested_model;

-- Review-level cost includes linked clarification/apply calls, including failed attempts.
SELECT * FROM ai_analysis_costs
WHERE created_at >= now()-interval '30 days'
ORDER BY created_at DESC;

-- Mean/P95 ONLY for fully priced recorded attempts; coverage columns must be checked.
SELECT status,count(*) AS reviews,
 count(*) FILTER (WHERE recorded_attempts=0) AS unmeasured_reviews,
 count(*) FILTER (WHERE recorded_attempts>0 AND recorded_estimate_usd IS NULL) AS incomplete_cost_reviews,
 count(recorded_estimate_usd) AS reference_priced_reviews,
 avg(recorded_estimate_usd) AS mean_reference_usd,
 percentile_cont(0.95) WITHIN GROUP (ORDER BY recorded_estimate_usd::double precision) AS p95_reference_usd
FROM ai_analysis_costs WHERE created_at>=now()-interval '30 days'
GROUP BY status;

-- Period-based platform spend subtotal per delivered review. Includes every AI operation,
-- failed calls and deleted work. This is a subtotal, not a complete cost or cohort LTV.
WITH spend AS (
 SELECT sum(estimate_nano_usd)::numeric/1000000000 AS observed_priced_subtotal_usd,
 count(*) FILTER (WHERE usage_state<>'known') AS pending_or_unknown_attempts,
 count(*) FILTER (WHERE usage_state='known' AND estimate_nano_usd IS NULL) AS known_unpriced_attempts
 FROM ai_usage_attempts WHERE started_at>=now()-interval '30 days'
), delivered AS (
 SELECT count(*) AS delivered_reviews FROM analyses
 WHERE status='completed' AND coalesce(analysis_completed_at,completed_at,created_at)>=now()-interval '30 days'
)
SELECT delivered_reviews,observed_priced_subtotal_usd,pending_or_unknown_attempts,known_unpriced_attempts,
 observed_priced_subtotal_usd/nullif(delivered_reviews,0) AS observed_priced_subtotal_per_delivered_usd
FROM spend CROSS JOIN delivered;

-- No underlying resume text/answers are present in these accounting records.
-- Unattributed, missing-usage and hard-killed calls remain visible for investigation.
SELECT operation,usage_state,unknown_reason,unpriced_reason,count(*)
FROM ai_usage_attempts WHERE started_at>=now()-interval '30 days'
GROUP BY 1,2,3,4 ORDER BY 5 DESC;
