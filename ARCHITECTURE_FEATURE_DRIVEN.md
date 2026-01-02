# Backend Architecture (Feature-Driven)

> Repo: Go backend for Resume Analyzer  
> Structure choice: **Feature-driven vertical slices** (`documents/`, `analyses/`) + small `shared/` for cross-cutting code.  
> Goal: Keep each feature self-contained while preserving clean boundaries: **transport → service → storage/clients**.

---

## 1) Guiding Principles

### 1.1 Vertical slices per feature
Each feature owns its:
- models (domain types for that feature)
- service (business logic)
- repository interface (persistence boundary)
- handler + DTOs (HTTP boundary)

### 1.2 Shared stays small
Only truly cross-cutting modules belong in `internal/shared/`:
- server bootstrapping + middleware
- config
- object store implementations (local/s3)
- DB connection/migrations
- LLM client + prompt registry
- extractors (pdf/docx)
- common error response helpers

### 1.3 Dependency direction
Handlers → Services → Repos/Stores/LLM/Extractor  
Never import Gin/HTTP into services/models.

---

## 2) Target Folder Structure

```
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
```

---

## 3) Feature #1: Documents (Upload + Current)

### 3.1 Endpoints
- `POST /api/v1/documents`  
  Upload a resume file (multipart `file`), save to object store, store metadata, return document id.
- `GET /api/v1/documents/current`  
  Fetch the latest uploaded document for the current user.

### 3.2 Data model (in-memory for MVP)
`DocumentsRepo` stores latest doc per user (map + mutex).  
Later we replace this with Postgres.

### 3.3 Flow
1. API receives multipart upload
2. Validates size (max 10MB) + file field presence
3. Saves file via `shared/storage/object.ObjectStore`
4. Creates `documents.Document` model
5. Stores document via `documents.DocumentsRepo`
6. Returns JSON response

---

## 4) Standard API Error Format

All errors return:

```json
{
  "error": {
    "code": "validation_error|not_found|unauthorized|internal",
    "message": "Human readable message",
    "details": [{"field":"file","issue":"required"}]
  }
}
```

Helpers live in `internal/shared/server/respond/`.

---

## 5) How to Add a New Feature (Checklist)

1. **Define the contract**
   - Routes + request/response DTOs for the feature

2. **Add/extend the feature models**
   - `internal/<feature>/model.go`

3. **Implement feature service**
   - `internal/<feature>/service.go` (orchestrates work)

4. **Define repo interfaces in feature**
   - `internal/<feature>/repo.go`

5. **Add storage/client implementations in shared**
   - object store, db, llm, extractor, etc.

6. **Wire handlers**
   - `internal/<feature>/handler.go`
   - Keep handlers thin; call service only

7. **Tests**
   - Service unit tests (mock repo/store)
   - Handler tests (httptest)
   - Schema validation tests for LLM outputs (later)

8. **Observability**
   - Add logs + metrics labels for new routes/operations

9. **Docs**
   - Update this document if flow changes

---

## 6) Near-Term Roadmap (Backend)

**Feature #1 (now):** Upload + current document (in-memory repo, local store)  
**Feature #2:** Persist documents in Postgres + list/history  
**Feature #3:** Analyses queue + worker + JSON-schema validated LLM output  
**Feature #4:** Auth (real OAuth/JWT), rate limiting, audit logs, billing hooks
