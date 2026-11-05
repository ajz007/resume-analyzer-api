# AI spending controls

Migration 0030 sets a shared **US$50 per UTC day** budget in PostgreSQL. Each HTTP or worker AI call reserves budget before it reaches OpenAI. A reservation covers the request and its maximum response with a 2x safety margin. Known usage settles at twice the standard reference estimate. Pending or unpriced usage keeps the full reservation for its original UTC day. This can pause the app before actual spend reaches US$50. The policy covers calls from the protected app deployments. The provider invoice can also include other callers, price changes and taxes.

The initial limits are 256 KiB per serialized text request, 32,768 completion tokens, and four provider attempts per operation, including retries and repairs. Lower response limits requested by the app still apply. The guard uses the GPT-6.1 Sol default service tier and blocks unsupported request shapes and models. If the guard cannot confirm capacity, it blocks the call. A guided answer stopped before any provider attempt keeps its included attempt.

## Rollout

Apply migration 0030 before starting the updated API and worker. Drain old API and worker instances before relying on the cap. Old binaries can still call the provider directly. Deploy the UI after the backend so failed reviews show the matching explanation. The migration uses the existing database and requires no new environment variables, AWS resources or scheduled jobs.

On the first day, known legacy attempts count at twice their reference estimate. Any pending or unpriced legacy attempt without a reservation pauses new calls for that UTC day unless an operator supplies a verified conservative allowance. Prefer letting these attempts settle or waiting for the next UTC day. Use the provider's records to reconcile uncertainty before changing the allowance. Keep migration 0030 tables and history during rollback.

## Operator queries

Run the inspection queries on the application database using an authorized account. Nano-USD values are converted to dollars for display.

```sql
SELECT calls_enabled, daily_limit_nano_usd / 1e9 AS daily_limit_usd,
       max_request_bytes, max_completion_tokens, max_attempts_per_operation
FROM ai_spend_policy;

SELECT day_utc, booked_nano_usd / 1e9 AS protected_booked_usd,
       legacy_uncertain_cover_nano_usd / 1e9 AS legacy_cover_usd, breach_detected
FROM ai_spend_days ORDER BY day_utc DESC LIMIT 7;

SELECT state, count(*) AS attempts, sum(booked_nano_usd) / 1e9 AS booked_usd
FROM ai_spend_holds
WHERE day_utc = (now() AT TIME ZONE 'UTC')::date
GROUP BY state;
```

To pause new calls, set `calls_enabled = false` on the singleton policy row. Set it back to true to resume admission under the same budget. Existing reservations remain. A call already admitted may finish. To change the budget, update `daily_limit_nano_usd` on the same row (one dollar is 1,000,000,000 nano-USD). Leave holds intact. A recorded reservation overrun stops further calls for that day and calls for a review of usage and pricing.

Compare measured usage with the [OpenAI model rates](https://developers.openai.com/api/docs/models/gpt-6.1-sol) and [Chat Completions limits](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) before changing model or request configuration.
