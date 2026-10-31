# V3 job-specific resume review

V3 stores one canonical set of source-anchored edits alongside the established analysis diagnostics. The UI can review changes, copy selected passages and export a Word resume from the same selected edits. Strong original passages remain unchanged. Scores continue to describe the submitted resume; edits and answers do not trigger rescoring.

## Enablement and compatibility

V3 is opt-in through `ANALYSIS_PROMPT_VERSION=v3` (or the existing supported request version). Existing prompt defaults and legacy report rendering remain. `ANALYSIS_MODEL` optionally selects the analysis model independently of other model consumers; otherwise `LLM_MODEL` is used. This change does not set production model or version environment variables. The OpenAI strict-schema path uses `ANALYSIS_REASONING_EFFORT` (default medium). Other providers do not gain answer-generation support through this change.

V3 reuse requires matching prompt version, model, provider and analysis version. Old non-V3 report reuse semantics remain. Update the analysis version when intentionally changing report behavior.

## Owner review API

- GET `/analyses/:id/review-session`: `{session, report}` including the assembled original edits and accepted answer additions.
- PUT `/analyses/:id/review-session/choices`: `{version, excludedIds}`.
- POST `/analyses/:id/review-session/answers`: `{version, requestId, requirementId, questionId, kind, answer, confirmed, excludedIds}`. The request ID is a UUID. `kind` is experience, none or not_sure. Experience answers require 20–3000 characters and explicit confirmation.
- POST `/analyses/:id/reviewed-resume`: selected `editIds` and `reviewVersion`. A current review version is required once session state exists. Stale selections return 409.

All session routes require ownership of a completed, unlocked, job-specific V3 analysis. Private session state is stored under `_reviewSession` in existing analysis-result JSONB, with owner-scoped optimistic CAS. No database migration is required. Original report data, source scores and raw analysis remain intact. Private answers and pending requests are excluded from standard result and public-share responses.

Experience answers reserve state before queueing and return 202 while pending. The existing worker handles `review_answer`; the queue message contains analysis/request identifiers, not answer text. The worker claims execution once before invoking a model. Duplicate delivery cannot repeat a paid model call. The UI polls until completion. Generation failures are recorded without automatic model repair/retry. Pending reservations expire after six minutes without automatic resubmission. None/not_sure is recorded without a model call.

The initial implementation permits six answer attempts per analysis. New generation is limited to one additional passage, clarification or no-change; it does not rewrite original edits. The completion limit for a delta is 8192 tokens. Inline completion is available when no queue is configured for local/test use.

## Evidence and export

Original edits require unique original-text anchors; whitespace and hyphen normalization does not permit changed words, numeric substrings or ambiguous matches. Derived section ordering is excluded from the provider schema. Bounded original-analysis repair retains the original resume and job description.

Answer additions require saved answer authorization, a verbatim supporting answer quote, original placement/context, a known requirement, supported numbers/ownership language and non-overlapping placement. These checks constrain generation but do not independently verify candidate statements. Missing qualifications can remain unresolved.

Word export applies selected edits to the original extracted source, preserving unedited content and reconstructing paragraphs/lists. For PDF uploads, original-file extraction must still match the analyzed source. Original visual styling and hyperlink annotations are not fully preserved. Failures to match or rebuild safely return an error rather than exporting guessed text.

## Validation and rollout

Run `go vet ./...` and `go test ./...`. Tests cover V3 validation, source safety, ordering, export, owner/payment access, optimistic conflicts, idempotency, queue completion and duplicate delivery. Fixtures use sanitized example text. A separately recorded fictional-data live model check confirmed one supported delta and export; it is not a broad answer-quality benchmark.

Deploy a worker that recognizes `review_answer` before enabling the updated HTTP/UI answer flow. Keep V3 opt-in until a staging check covers queue completion, payment access, reload persistence and export against the intended model configuration. No merge or deployment is performed by opening the draft PR.
