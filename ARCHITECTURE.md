# Backend Architecture

> Repo: `resume-backend` (Go)  
> Goal: Production-ready API + worker for Resume Analyzer (upload → analyze → results/history), with clear layering, testability, and predictable changes.

---

## 1. Current State (from existing project)

- Single binary at `cmd/main.go`
- `net/http` handlers registered directly in `main`
- OpenAI call is performed inside the handler
- No persistence (no DB), no job orchestration, basic CORS

This is a great starting point for an MVP, but to scale features safely we’ll introduce **structure**:
- Separate **HTTP API** from **business logic**
- Introduce **domain + services** layer
- Add **storage** (DB + object store)
- Add **async jobs/worker** for analysis
- Add **versioned prompts** and **schema validation**

---

## 2. Target Architecture (High Level)

### Components

1. **API Service (Go)**
   - Auth middleware (OAuth/JWT integration point)
   - Upload/resume endpoints
   - Analysis orchestration (enqueue jobs, return status/results)
   - History endpoints
   - Validation, rate limiting, CORS, logging

2. **Worker (Go)**
   - Pull queued analysis jobs
   - Extract text from PDF/DOCX
   - Normalize + compute deterministic stats
   - Call LLM with a versioned prompt
   - Validate JSON output against a JSON Schema
   - Store results & metrics

3. **Storage**
   - Postgres: documents, analyses, jobs, prompt_versions, usage metrics
   - Object store: S3/GCS/minio/local for uploaded files

---

## 3. Proposed Folder Structure (recommended)

```
resume-backend/
  cmd/
  api/
    main.go                       # API server (Gin)
  worker/
    main.go                       # background worker (later)

internal/
  documents/
    model.go                      # Document + feature errors
    repo.go                       # DocumentsRepo interface
    service.go                    # DocumentService
    handler.go                    # Gin handlers
    dto.go                        # request/response DTOs

  analyses/                       # later feature
    model.go
    repo.go
    service.go
    handler.go
    processor/
      processor.go

  shared/
    config/
      config.go

    server/
      router.go                   # gin.Engine, routes
      middleware/
        auth.go                   # X-User-Id (MVP)
        cors.go
        request_id.go
        logging.go
        recovery.go
        ratelimit.go              # optional later
      respond/
        errors.go                 # standard error format + helpers
        json.go

    storage/
      object/
        store.go                  # ObjectStore interface
        local/
          store.go                # local disk implementation
        s3/
          store.go                # later

      db/
        db.go                     # postgres connect (later)
        migrate.go                # migrations runner (later)

    llm/
      client.go                   # interface
      openai/
        client.go
      prompts/
        v1/
          prompt.tmpl
          schema.json
          metadata.json

    extract/
      extractor.go
      pdf.go
      docx.go

    telemetry/
      logger.go
      metrics.go
      tracing.go

    util/
      hash.go
      json.go
      sanitize.go
      time.go

migrations/                       # later
docs/
  ARCHITECTURE.md
  go.mod
  go.sum
```

### Why this structure works
- `cmd/*` is only bootstrapping (no business logic).
- `internal/*` holds everything application-specific and non-exported.
- `domain` is clean and testable.
- `services` orchestrate rules and workflows.
- `transport/http` is thin: parse/validate → call service → return response.
- LLM prompts are **versioned artifacts**, not hardcoded strings.

---

## 4. Key Design Rules (non-negotiable)

### 4.1 Dependency direction
HTTP handlers → services → repos/clients  
Never the other way around.

### 4.2 Interfaces at boundaries
Define interfaces at the layer that *uses* them:
- `services` defines `DocumentsRepo`, `ObjectStore`, `JobQueue`, `LLMClient`, `Extractor`
- `storage/*`, `llm/*`, `extract/*` provide implementations

### 4.3 Domain is dependency-free
No `net/http`, no OpenAI SDK, no SQL in `domain/`.

### 4.4 Prompts are versioned
- Each analysis stores `prompt_version`
- Prompt output is validated against `llm/prompts/<version>/schema.json`
- Prompt changes are backwards-compatible by versioning, not by editing the old prompt

---

## 5. Request Flow (end-to-end)

### Upload (`POST /api/documents`)
1. API validates file type/size
2. Stores file in object store → returns `storage_key`
3. Inserts `documents` row in Postgres
4. Returns `documentId` + metadata

### Analyze (`POST /api/documents/:id/analyze`)
1. API creates `analyses` row with `status=queued` + `prompt_version=v1`
2. Enqueues job `{analysisId, documentId}`
3. Returns `{analysisId, status:"queued"}`

### Worker
1. Dequeues job
2. Loads document metadata + file
3. Extracts text
4. Normalizes + computes stats (pages approx, sections, bullets count, numeric evidence count)
5. Calls LLM (1 call ideally) with `{resume_text, resume_stats, job_desc?}`
6. Validates JSON output against schema
7. Post-process (dedupe, clamp scores, fill unknowns safely)
8. Stores `result_json`, marks analysis `completed`

### Results (`GET /api/analyses/:id`)
Returns status + result when ready.

---

## 6. API Conventions

### 6.1 Versioning
Prefer URL versioning:
- `/api/v1/documents`
- `/api/v1/analyses`

If you’re early, you can keep `/api/*` and introduce v1 later, but schema/prompt versioning is still required.

### 6.2 Error format (consistent)
All errors return:
```json
{
  "error": {
    "code": "validation_error|not_found|unauthorized|internal",
    "message": "Human readable message",
    "details": [{"field":"file","issue":"too_large"}]
  }
}
```

### 6.3 Idempotency
- `POST /documents/:id/analyze` can be idempotent by returning latest queued/running analysis for that doc unless `force=true`.

---

## 7. Configuration

### 7.1 Config source
- Environment variables only (12-factor)
- Use `.env` only for local dev

### 7.2 Required env vars (initial)
- `PORT`
- `DATABASE_URL`
- `OPENAI_API_KEY`
- `OBJECT_STORE=local|s3`
- `LOCAL_STORE_DIR` (if local)
- `CORS_ALLOW_ORIGINS` (comma-separated)
- `LOG_LEVEL`

---

## 8. Observability

### Logging
- Structured JSON logs
- Include `request_id`, `user_id` (hashed), `document_id`, `analysis_id`

### Metrics (minimum)
- request latency by route
- worker job latency
- LLM token usage + error rates
- queue depth

### Tracing (optional early)
- OpenTelemetry spans for API request + worker job

---

## 9. Security & Privacy

- Never log raw resume text
- Store extracted text only if required; if stored, encrypt at rest or restrict access
- Rate limit analyze endpoints
- Validate uploads (MIME + magic bytes)
- Limit file size (e.g., 5–10MB MVP)
- Consider PII masking in extracted text if saved

---

## 10. How to Add a New Feature (Checklist)

When you add a feature, follow this order:

1. **Define the contract**
   - API endpoint(s) + request/response DTOs
   - Update OpenAPI/Swagger (if used)

2. **Domain changes**
   - Add/extend domain models (no external deps)

3. **Service logic**
   - Implement feature in `internal/services/*`
   - Add interfaces needed for persistence/clients

4. **Storage**
   - Add migration(s) in `/migrations`
   - Implement repo method(s) in `internal/storage/repo/*`

5. **Transport**
   - Add handler + validation in `internal/transport/http/handlers/*`
   - Keep handlers thin (no SQL/LLM here)

6. **Worker changes (if async)**
   - Add job payload + processor updates
   - Ensure retries + idempotency

7. **Tests**
   - Unit test services (mock repos/clients)
   - Handler tests (httptest)
   - Repo tests (optional with testcontainers)
   - Schema validation tests for prompt output

8. **Observability**
   - Add logs + metrics labels
   - Add dashboards/alerts later, but emit metrics now

9. **Docs**
   - Update `ARCHITECTURE.md` if it changes flow
   - Add `docs/decisions/*.md` for major choices (ADR style)

---

## 11. Testing Strategy

### Unit tests (fast)
- `services/*` with mocks
- `llm/prompt` rendering correctness
- `schema validation` (given sample output must pass)

### Integration tests (medium)
- HTTP handlers with in-memory deps or dockerized Postgres
- Object store local implementation

### End-to-end (later)
- Upload → analyze → results on a real queue/worker

---

## 12. Prompt Management (Critical)

- Store prompt files under `internal/llm/prompts/<version>/`
- Each prompt version includes:
  - `prompt.tmpl`
  - `schema.json`
  - `metadata.json` (notes, date, model defaults)

**Rule:** Never change an old prompt in place. Create `v2`.

---

## 13. Minimal Migration Plan from Current Code

1. Move current `cmd/main.go` → `cmd/api/main.go`
2. Create `internal/server` and route `/customize` as a handler
3. Wrap OpenAI client behind `internal/llm` interface
4. Add `services/customize_service.go` (moves logic out of handler)
5. Later: replace `/customize` with the new upload/analyze endpoints

---

## 14. Definition of Done (for a backend feature)

A feature is done when:
- Endpoint exists + validated inputs
- Service logic is covered by unit tests
- DB migration applied (if needed)
- Logs + metrics added for the main path
- Error responses follow the standard format
- Docs updated if flow or interfaces changed

---

## 15. Tech Choices (recommended defaults)

- Router: `chi` (simple, stdlib-friendly) or `net/http` with a tiny router
- DB: Postgres
- Migrations: `golang-migrate`
- Queue MVP: Postgres-backed queue (single table) → later Redis/SQS
- LLM client: wrap `go-openai` behind interface
- JSON Schema validation: validate in worker before persisting result

---

### Appendix: Naming

- **Handlers**: `documents.go`, `analyses.go`
- **Services**: `DocumentService`, `AnalysisService`
- **Repos**: `DocumentsRepo`, `AnalysesRepo`
- **IDs**: always UUIDs
- **Time**: UTC everywhere

