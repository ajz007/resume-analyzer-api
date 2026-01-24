# Phase 1 Definition of Done (DoD) — Resume Analyzer

Scope: Phase 1 only (upload → analyze → results). Phase 2 features (DOCX/apply/generated-resumes) are excluded from the Phase 1 test gate via build tag `phase2`.

---

## 1) API DoD (curl examples)

- [ ] **Upload accepts PDF/DOC/DOCX ≤5MB**  
  Pass: `POST /api/v1/documents` returns **201** and `mimeType` matches upload for a ≤5MB PDF/DOC/DOCX.  
  Fail: valid type/size rejected or >5MB accepted.  
  ```bash
  export API_BASE="http://localhost:8080"
  export GUEST_ID="test-guest"
  curl -sS -X POST "$API_BASE/api/v1/documents" \
    -H "X-Guest-Id: $GUEST_ID" \
    -F "file=@./path/to/sample.pdf"
  ```

- [ ] **Upload rejects oversize (>5MB)**  
  Pass: HTTP 4xx and error.code=validation_error (or payload_too_large).  
  Manual step: upload a file >5MB and verify status.

- [ ] **Upload rejects unsupported type**  
  Pass: `.txt` upload returns 4xx (ideally 415) and error.details mentions mime/type.  
  ```bash
  curl -i -X POST "$API_BASE/api/v1/documents" \
    -H "X-Guest-Id: $GUEST_ID" \
    -F "file=@./README.md"
  ```

- [ ] **Start analysis enqueues async job**  
  Pass: `POST /api/v1/documents/:id/analyze` returns **202** with `analysisId` and `status` (`queued` or `processing`).  
  ```bash
  DOC_ID="<documentId-from-upload>"
  curl -sS -X POST "$API_BASE/api/v1/documents/$DOC_ID/analyze" \
    -H "X-Guest-Id: $GUEST_ID" \
    -H "Content-Type: application/json" \
    -d '{"jobDescription":"'"$(python - <<'PY'
print("a"*300)
PY
)"'","promptVersion":"v2_1"}'
  ```

- [ ] **Polling shows status progression**  
  Pass: `queued → processing → completed` (or `failed`).  
  ```bash
  ANALYSIS_ID="<analysisId-from-start>"
  curl -sS "$API_BASE/api/v1/analyses/$ANALYSIS_ID" \
    -H "X-Guest-Id: $GUEST_ID"
  ```

- [ ] **Completed analysis includes result**  
  Pass: `result` present when `status=completed`.  

- [ ] **Failed analysis is actionable**  
  Pass: `errorMessage` present when `status=failed`.  
  Manual step: set invalid `OPENAI_API_KEY`, start analysis, and verify failure payload.

---

## 2) DB DoD (migrations, constraints, indexes)

- [ ] **Migrations applied on startup**  
  Pass: app logs show migrations applied and DB used (no memory fallback).  
  Manual step: start server with valid `DATABASE_URL`.

- [ ] **Goose migration status**  
  Pass: all migrations are applied.  
  ```bash
  goose -dir internal/shared/storage/db/migrations postgres "$DATABASE_URL" status
  ```

- [ ] **documents table columns present**  
  Pass: includes `id`, `user_id`, `mime_type`, `size_bytes`, `storage_key`.  
  ```bash
  psql "$DATABASE_URL" -c "\d+ documents"
  ```

- [ ] **analyses table columns present**  
  Pass: includes `status`, `result`, `job_description`, `error_message`, `started_at`, `completed_at`, `updated_at`.  
  ```bash
  psql "$DATABASE_URL" -c "\d+ analyses"
  ```

- [ ] **FK + status constraint present**  
  Pass: constraints exist.  
  ```bash
  psql "$DATABASE_URL" -c "SELECT conname FROM pg_constraint WHERE conname IN ('analyses_document_id_fkey','analyses_status_check');"
  ```

- [ ] **Index for per-user document queries exists**  
  Pass: `idx_documents_user_created_at` exists.  
  ```bash
  psql "$DATABASE_URL" -c "SELECT indexname FROM pg_indexes WHERE indexname='idx_documents_user_created_at';"
  ```

---

## 3) Security/Isolation DoD (guest + auth)

- [ ] **Guest identity required**  
  Pass: missing `Authorization` and `X-Guest-Id` returns **401**.  
  ```bash
  curl -i "$API_BASE/api/v1/documents/current"

  ```

- [ ] **Guest isolation on analyses read**  
  Pass: guest B cannot fetch guest A analysis (404).  
  ```bash
  curl -i "$API_BASE/api/v1/analyses/$ANALYSIS_ID" -H "X-Guest-Id: guest-b"
  ```

- [ ] **Guests cannot list history**  
  Pass: `GET /api/v1/analyses` returns **401** for guests.  
  ```bash
  curl -i "$API_BASE/api/v1/analyses" -H "X-Guest-Id: $GUEST_ID"
  ```

---

## 4) UI DoD (manual checks)

- [ ] **JD min length enforced in UI**  
  Pass: UI blocks submit for <300 chars with clear error.

- [ ] **Upload size/type guard in UI**  
  Pass: UI blocks non PDF/DOC/DOCX and >5MB.

- [ ] **Polling UX states**  
  Pass: UI shows `queued/processing/completed/failed` transitions.

- [ ] **Failed analysis error shown**  
  Pass: UI displays `errorMessage` on failure.

---

## 5) Observability DoD (logs/errors)

- [ ] **Request logs include request_id + status**  
  Pass: `request.complete` log includes `request_id`, `status`, `path`.  
  Manual step: hit any endpoint and inspect stdout.

- [ ] **Responses include X-Request-Id**  
  Pass: response header includes `X-Request-Id`.  
  ```bash
  curl -i "$API_BASE/api/v1/health" -H "X-Guest-Id: $GUEST_ID"
  ```

- [ ] **Failed analysis logs are sanitized**  
  Pass: error message is single-line and ≤500 chars.  
  Manual step: force LLM failure and inspect logs.

---

## 6) Test DoD (commands)

- [ ] **Phase 1 test gate passes**  
  ```bash
  go test ./...
  ```

- [ ] **Phase 2 tests compile behind tag**  
  ```bash
  go test -tags phase2 ./...
  ```

- [ ] **Variability controls present (temperature=0 where supported + list normalization)**  
  Pass: temperature set to 0 and list normalization applied.  
  ```bash
  rg -n "Temperature.*0" internal/llm/openai/client.go
  rg -n "normalizeResultOrdering" internal/analyses/service.go
  ```

---

## 7) Release checklist (env vars, config, smoke test)

- [ ] **Required env vars set**  
  Pass: `ENV`, `PORT`, `LLM_PROVIDER`, `LLM_MODEL`, and `OPENAI_API_KEY` (when using OpenAI).  

- [ ] **Production DB required**  
  Pass: `ENV=production` with `DATABASE_URL` set; Fail otherwise.

- [ ] **Object store configured**  
  Pass: `OBJECT_STORE=local` uses `LOCAL_STORE_DIR`, or `OBJECT_STORE=s3` has `AWS_REGION` + `S3_BUCKET`.

- [ ] **Guest smoke test**  
  Pass: upload → analyze → result succeeds for a guest.  
  ```bash
  set -euo pipefail
  export API_BASE="http://localhost:8080"
  export GUEST_ID="test-guest"
  export DOC_ID=""
  export ANALYSIS_ID=""

  UPLOAD_JSON=$(curl -sS -X POST "$API_BASE/api/v1/documents" \
    -H "X-Guest-Id: $GUEST_ID" \
    -F "file=@./fixtures/golden_resume.pdf" \
    -F "filename=golden_resume.pdf")
  DOC_ID=$(echo "$UPLOAD_JSON" | jq -r '.documentId')

  ANALYZE_JSON=$(curl -sS -X POST "$API_BASE/api/v1/documents/$DOC_ID/analyze" \
    -H "X-Guest-Id: $GUEST_ID" \
    -H "Content-Type: application/json" \
    -d @<(jq -n --rawfile jd ./fixtures/golden_job_description.txt '{
      jobDescription: $jd,
      promptVersion: "v2_1"
    }'))
  ANALYSIS_ID=$(echo "$ANALYZE_JSON" | jq -r '.analysisId')

  for i in {1..30}; do
    STATUS=$(curl -sS "$API_BASE/api/v1/analyses/$ANALYSIS_ID" \
      -H "X-Guest-Id: $GUEST_ID" | jq -r '.status')
    echo "status=$STATUS"
    [[ "$STATUS" == "completed" ]] && break
    [[ "$STATUS" == "failed" ]] && exit 1
    sleep 2
  done

  curl -sS "$API_BASE/api/v1/analyses/$ANALYSIS_ID" \
    -H "X-Guest-Id: $GUEST_ID" | jq -e '.result'
  ```
