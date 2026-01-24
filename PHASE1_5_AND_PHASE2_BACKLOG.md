# Phase 1.5 and Phase 2 Backlog

## Phase 1.5 (Hardening / Low Risk)

1) **Validate FK and status CHECK constraints**
- Description: Ensure DB constraints are created and validated (including NOT VALID → VALID) for analyses foreign key and status enum.
- Why it matters: Prevents invalid analysis rows and strengthens data integrity for async lifecycle.
- Risk level: Low
- Acceptance criteria:
  - `SELECT convalidated FROM pg_constraint WHERE conname IN ('analyses_document_id_fkey','analyses_status_check');` returns `t` for both.
  - Attempts to insert invalid `status` or `document_id` fail with constraint errors.

2) **Stuck-analysis sweeper (processing → failed_timeout)**
- Description: Periodic job that marks analyses stuck in `processing` past a timeout as `failed` with an explicit error message.
- Why it matters: Prevents indefinite polling and enables actionable failure states.
- Risk level: Medium
- Acceptance criteria:
  - Any analysis in `processing` longer than timeout is transitioned to `failed`.
  - Error message includes a timeout reason.
  - Manual test: create a processing record and verify it is marked failed after timeout.

3) **Retry attempt tracking (retry_of / attempt_count)**
- Description: Track retries for analysis attempts to avoid silent loops and improve debugging.
- Why it matters: Improves observability and makes failures traceable.
- Risk level: Medium
- Acceptance criteria:
  - Each retry increments `attempt_count` and links to `retry_of`.
  - API exposes attempt metadata for the analysis.
  - Queries show the latest attempt for a document.

4) **Rate-limit analyze endpoint per guest**
- Description: Apply per-guest limits to `POST /api/v1/documents/:id/analyze`.
- Why it matters: Protects LLM costs and stabilizes the system under bursty guest usage.
- Risk level: Medium
- Acceptance criteria:
  - Exceeding the limit returns HTTP 429 with a clear error payload.
  - Limits are enforced per guest ID, not globally.
  - Documented limits in release notes.

5) **Improve observability (metrics for queued/processing/failed)**
- Description: Export counters/gauges for analysis status transitions and current queue depth.
- Why it matters: Enables operational visibility and faster incident response.
- Risk level: Low
- Acceptance criteria:
  - Metrics report counts for `queued`, `processing`, `completed`, `failed`.
  - Dashboard or log output demonstrates metrics collection.

6) **Background job health check**
- Description: Add a lightweight health indicator that verifies the async worker path is functioning.
- Why it matters: Detects analysis pipeline regressions early.
- Risk level: Low
- Acceptance criteria:
  - Health endpoint reports worker readiness or degraded status.
  - Manual test shows healthy status when worker can process jobs.

---

## Phase 2 (Product Expansion)

1) **Resume rewrite workflows**
- Description: User-initiated rewrite flows that transform resume content based on analysis results.
- Dependencies: Phase 1 analysis stability; prompt/templates defined; storage for rewritten artifacts.
- Scope boundary: Does not include DOCX rendering or template management.

2) **DOCX rendering & ATS-safe output**
- Description: Generate ATS-safe DOCX output from rewrite results.
- Dependencies: Rewrite workflows; validated DOCX renderer; output validation checks.
- Scope boundary: Does not include template versioning or premium template library.

3) **Apply flow stabilization**
- Description: Harden apply pipeline to consistently produce valid outputs and reliable status tracking.
- Dependencies: DOCX rendering; stable analysis results; storage for generated resumes.
- Scope boundary: Does not include multi-variant generation or bulk batch apply.

4) **Versioned resume templates**
- Description: Template metadata, selection, and versioned rendering.
- Dependencies: DOCX rendering; template storage and selection APIs.
- Scope boundary: Does not include a public template marketplace or designer tooling.

5) **User accounts (OAuth)**
- Description: Persistent user accounts with OAuth login and saved history.
- Dependencies: Auth provider configuration; user data model and migrations.
- Scope boundary: Does not include enterprise SSO or admin roles.

6) **Billing & quotas**
- Description: Metered usage, plan limits, and payment integration.
- Dependencies: User accounts; usage tracking; billing provider integration.
- Scope boundary: Does not include invoicing workflows or enterprise contracts.

7) **Phase 2 test strategy (no build tags)**
- Description: Full test coverage for Phase 2 features without build-tag gating.
- Dependencies: Stable Phase 2 APIs; deterministic fixtures; CI environment setup.
- Scope boundary: Does not include performance/load testing beyond basic smoke tests.

---

## Out of Scope (Explicit)

- Multi-language resume parsing beyond current extraction capabilities.
- Real-time collaborative editing in the UI.
- Native mobile apps.
- Third-party ATS integrations.
- Enterprise governance features (audit logs, RBAC, SCIM).
