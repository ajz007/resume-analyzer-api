# Hardening Guide

This document describes the analysis lifecycle, failure codes, retry rules, idempotency, and debugging guidance.

## Status lifecycle

Analyses move through these states:

- `queued` -> initial state after creation.
- `processing` -> background worker started.
- `completed` -> result normalized and persisted.
- `failed` -> unrecoverable error recorded.

Status transitions are logged via `analysis.status` events with:

- `status_transition`: `queued->processing`, `processing->completed`, or `processing->failed`.
- `request_id`, `user_id`, `document_id`, `analysis_id`.

## Failure codes

Failures are normalized to these codes:

- `VALIDATION_ERROR`
- `LLM_TIMEOUT`
- `LLM_SCHEMA_MISMATCH`
- `STORAGE_ERROR`
- `INTERNAL_ERROR`

On failure, API responses for `GET /api/v1/analyses/:id` include:

- `errorCode`
- `errorMessage`
- `retryable`

## Retry rules

### LLM retries

The LLM client retries **once** with exponential backoff **only** for:

- timeouts
- HTTP 5xx
- network errors

No retries for:

- HTTP 4xx
- schema mismatch / parsing errors
- validation errors

### Client retries

If an analysis is `failed`, clients must set `retry=true` (query) or `X-Retry-Analysis: true` to create a new analysis for the same document.

## Idempotency rules

`POST /api/v1/documents/:id/analyze` is idempotent per document:

- If an analysis for the document is `queued` or `processing`, it is reused.
- If an analysis is `completed`, the existing result is returned.
- If an analysis is `failed`, a retry is only allowed with explicit retry flags.

## Debugging steps (trace one analysis via request_id)

1. Find the initial `request_id` from the client response header `X-Request-Id`.
2. Search logs for `request.complete` with the `request_id` to confirm inbound request metadata.
3. Search for `analysis.status` logs with the same `request_id` to follow transitions and durations.
4. Use `analysis_id` from logs to fetch the analysis:
   - `GET /api/v1/analyses/:id`
5. If `failed`, review:
   - `errorCode`, `errorMessage`, `retryable`
   - `analysis_raw` and `analysis_result` in the database (if available).

Tip: if you only have `document_id`, you can find the latest analysis for that document in storage and then trace logs by `analysis_id`.
