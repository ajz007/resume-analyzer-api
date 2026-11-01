# AI usage measurement

## Deployment scope

Migration 0029 adds ai_usage_attempts and two internal read-only reporting views to the existing Neon database. Updated API and worker code record normal OpenAI traffic automatically when PostgreSQL is configured. There are no new environment variables, AWS resources, schedules, external reporting calls, public cost endpoints or paid experiments. Existing model selection, prompts, allowances, prices and provider retry policies are preserved.

Apply the migration before deploying both API and worker updates. Verify the runtime database role can select/insert/update the new accounting table. On code rollback, retain the accounting records and views; migration Down deliberately refuses to delete history. Only new provider calls are measured; historical reports are not backfilled or assigned a guessed cost.

The previous optional recovery schedule remains undeployed unless the owner separately chooses to activate it. This change does not deploy or enable it.

## What is recorded

Each physical provider HTTP exchange has a fresh attempt UUID. An internal operation UUID groups the primary request, transient retries, parameter retries, JSON repairs and validation repairs. Analysis/review-answer/apply calls link to the analysis UUID; answer requests, asynchronous generation jobs and tailoring sources also carry their relevant internal object UUID. Synchronous generation has an operation UUID even before the resulting resume exists. Missing attribution is labelled unattributed, not silently assigned to a user or review.

The ledger contains provider/requested/returned model, provider request/response identifiers, reported service tier, HTTP outcome, elapsed provider-response time, token counts, stage/retry tags and a reference estimate when supported. It does not store resumes, job descriptions, answers, names, email addresses, API keys, request/response bodies or raw error messages. UUIDs remain internal accounting metadata; access is through the existing private database role, not a user-facing endpoint.

A small pending record is written before sending. Finalization occurs when the response body is consumed or closed. A hard kill can leave pending usage, and an incomplete body, timeout, missing usage, invalid counts or capture overflow remains unknown. Unknown cost is NULL, never an inferred zero. Usage remains recorded when a model response is refused, truncated or later rejected by application validation. HTTP response outcome is not the same as successful review delivery; use the linked analysis status when evaluating outcomes.

All OpenAI analysis and prompt clients are instrumented, including main reviews, clarification answers, apply generation, resume generation, tailoring and their existing retries/repairs. The recorder is installed at bootstrap before concurrent use. Reconfiguration replaces a previous meter rather than double counting it.

## Cost estimates and limitations

The initial reference rate is the standard global text price for exactly gpt-6.1-sol, verified on 1 November 2026. It uses USD per million tokens: uncached input 2.00, cached input 0.10, cache writes 2.50 and output 10.00. For input above 272,000 tokens, the full request uses doubled input/cache rates and 1.5x output. The estimate stores a pricing-version label and integer nano-USD; reasoning tokens are a subset of completion tokens and are not added twice. Optional cache/reasoning details default to zero according to the API schema. [Official model rate card](https://developers.openai.com/api/docs/models/gpt-6.1-sol), [Chat Completions usage schema](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create).

These are reference estimates, not OpenAI invoice amounts. They assume standard global processing and exclude regional premiums, taxes, account-specific discounts or adjustments. An unknown model/snapshot, requested/returned model mismatch, missing/non-default service tier, non-text usage or unknown token usage remains unpriced. Raw known token counts are retained even if no supported rate applies. Rates do not update automatically; review/version them before relying on new pricing. No model is substituted and no provider billing endpoint is queried.

This is measurement, not a spending cap. If accounting storage fails, the existing provider request proceeds; accounting errors never trigger an AI retry. Writes have independent one-second timeouts and emit ai.usage.record_gap with attempt ID and write phase, without raw errors. A successful final write can recover a failed initial write. If both writes fail, the database alone cannot establish full coverage; check existing logs for record_gap events before pricing from the samples. Pending/unknown rows also count as incomplete, even if the provider may not have billed them.

The observer captures at most 2 MiB of response bytes in process memory for metadata parsing and passes the original body through unchanged. Larger responses remain unpriced rather than being truncated for the caller. Overhead is up to two bounded database writes per actual HTTP exchange plus small metadata-row storage and bounded memory; this is not a zero-cost or zero-latency claim. No polling or recurring reads are introduced.

## Using the measurements

Run the read-only queries in ai-usage-reports.sql manually in the existing SQL editor and select the deployment/cohort date you intend to inspect. ai_usage_daily shows all recorded operations, known tokens, partial priced subtotals and incomplete/unpriced attempt counts. ai_analysis_costs includes linked main/answer/apply calls across all outcomes, including deleted work, so failed work does not disappear from review accounting. Reports with no records have unknown cost, not zero cost.

Mean/P95 estimates apply only to fully reference-priced recorded attempts. Review the coverage counts and record_gap logs alongside them; do not present that measured subset as an unconditional actual-cost average. The period-based platform subtotal per delivered review includes other AI operations and failed work, but remains a subtotal if there are gaps. Provider spend is only one part of pricing: payment fees, refunds, infrastructure, acquisition and support remain outside these views.

## Validation and remaining work

Local tests use fake transports/local HTTP servers only. They verify token parsing, cache writes, long-context boundaries, reasoning inclusion, unknown/unpriced cases, refusal/truncation, JSON/parameter/transient retries, actual analysis linkage, unchanged response bytes, oversized/incomplete bodies, context cancellation, accounting failure isolation, concurrent attempt numbering and recorder replacement. PostgreSQL tests apply all migrations in isolated disposable local schemas and verify pending durability, idempotent finalization, recovery of a missing start write, incomplete cost reporting, unmeasured historical reports and read-only operator queries.

Database tests require AI_USAGE_TEST_DATABASE_URL pointing to a disposable loopback-only PostgreSQL database and skip without it. No real OpenAI, SQS, payment, cloud-resource or production-database action was performed. The next increment is input/output/retry limits and shared daily spending protection, with an explicit policy for accounting outages and unknown provider usage. Refund reconciliation remains a separate paid-launch requirement.
