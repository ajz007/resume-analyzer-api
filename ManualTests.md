# Manual API Testing: Document Upload + Analyze

*(Resume Analyzer — API-level checks + “Golden Doc” validation checklist)*

> **Update:** All API calls below use **guest mode** via
> `-H "X-Guest-Id: <guest-id>"`
> (as per our earlier manual tests).
> **No Authorization header is used.**

---

## 0) Prereqs

### Environment variables

```bash
export API_BASE="http://localhost:8080"   # change if deployed
export GUEST_ID="test-user"               # any stable guest identifier
```

### Helper header (used in ALL curls)

```bash
GUEST_HEADER=(-H "X-Guest-Id: $GUEST_ID")
```

---

## 1) Health / Version (sanity check)

**GET** `/healthz`

```bash
curl -sS "$API_BASE/healthz" \
  "${GUEST_HEADER[@]}"
```

Expected:

* HTTP 200
* Body indicates service is healthy

---

## 2) Upload Document (PDF/DOCX)

### 2.1 Upload endpoint

**POST** `/api/v1/documents`
Multipart upload with a file.

```bash
curl -i -X POST -H "X-Guest-Id: test-user"
   -F "file=@/d/Ajit/Gigs/test_resumes/Test_Resume_2025_Upload_ResumeScanner.docx" http://localhost:8080/api/v1/documents
```
```bash
curl -sS -X POST "$API_BASE/api/v1/documents" \
  "${GUEST_HEADER[@]}" \
  -F "file=@./fixtures/golden_resume.pdf" \
  -F "filename=golden_resume.pdf"
```

Expected:

* HTTP **201** (or **200**, depending on implementation)
* Response JSON includes:

  * `document_id` (UUID)
  * `status` (e.g. `uploaded`)
  * `created_at`

Save the ID:

```bash
export DOC_ID="<paste_document_id_here>"
```

---

### 2.2 Upload negative tests

| Scenario              | Expected                     |
| --------------------- | ---------------------------- |
| Missing file          | `400 Bad Request`            |
| Unsupported file type | `415 Unsupported Media Type` |
| File too large        | `413 Payload Too Large`      |
| Empty file            | `400 Bad Request`            |

---

## 3) Analyze (Apply) Against a Job Description

### 3.1 Start analysis

**POST** `/api/v1/documents/{document_id}/analyze`

```bash
curl -i -X POST \
  -H "X-Guest-Id: test-user" \
  -H "Content-Type: application/json" \
  -d '{"promptVersion":"v2_3"}' \
  http://localhost:8080/api/v1/documents/<DOCUMENT_ID>/analyze

```
```bash
curl -sS -X POST "$API_BASE/api/v1/documents/$DOC_ID/analyze" \
  "${GUEST_HEADER[@]}" \
  -H "Content-Type: application/json" \
  -d '{
    "job_title": "Senior Backend Engineer (Java/Spring)",
    "job_description": "We need 8+ years of Java, Spring Boot microservices, REST APIs, SQL (Postgres/Oracle), Kubernetes/Docker, CI/CD. Cloud experience on AWS/GCP is a plus. Strong communication and mentoring.",
    "options": {
      "output_format": "docx",
      "ats_safe": true,
      "include_cover_letter": false
    }
  }'
```

Expected:

* HTTP **202 Accepted**
* Response JSON includes:

  * `analysis_id`
  * `status: queued | processing`

Save:

```bash
export ANALYSIS_ID="<paste_analysis_id_here>"
```

---

### 3.2 Poll analysis status

**GET** `/api/v1/documents/{document_id}/analyses/{analysis_id}`

```bash
curl -s \
  -H "X-Guest-Id: test-user" \
  http://localhost:8080/api/v1/analyses/<ANALYSIS_ID> | jq

```

Expected progression:

```
queued → processing → succeeded
```

(or `failed` with an error object)

---
### 3.3 Apply analysis to resume

```bash
curl -i -X POST \
  -H "X-Guest-Id: test-user" \
  http://localhost:8080/api/v1/analyses/<ANALYSIS_ID>/apply

```
---

### 3.4 Download generated DOCX


```bash
curl -L --fail-with-body \
  -H "X-Guest-Id: test-user" \
  -o out/generated.docx \
  http://localhost:8080/api/v1/generated-resumes/<GENERATED_RESUME_ID>/download

```

Expected:

* HTTP **200**
* `Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document`
* Non-zero file size

---

## 4) Golden Doc Strategy (Deterministic Validation)

Avoid byte-level DOCX comparison. Validate **structure + signals** instead.

### 4.1 Golden fixtures

```
fixtures/
├── golden_resume.pdf
├── golden_job_description.txt
├── golden_expected.json
└── golden_expected_doc_rules.md
```

---

## 5) Golden Assertions

### 5.1 JSON result assertions

Validate using `jq`:

```bash
jq '.match_score >= 70' out/result.json
jq '.skills_found[]' out/result.json | grep -E "Java|Spring Boot|Kubernetes"
```

Typical expectations:

* Required skills detected (Java, Spring Boot, REST, SQL, Docker, K8s)
* `match_score` above threshold
* Gaps list stable and explainable
* Minimum N bullet rewrites in Experience section

---

### 5.2 DOCX Output – Definition of Done

**ATS & Word Safety**

* Opens in MS Word without repair prompt
* No tables used for layout
* No text boxes or floating elements
* Single-column body
* Standard fonts only

**Content**

* Header with name + contact
* Summary present
* Skills, Experience, Education present
* No template tokens (`{{ }}`, `TBD`, `TODO`)

**Delivery**

* Correct content-type
* Clear JSON error on failure (no partial DOCX)

---

## 6) Minimal Golden Inputs

### `fixtures/golden_job_description.txt`

```text
Senior Backend Engineer (Java/Spring)
Must have: 8+ years Java, Spring Boot microservices, REST APIs,
SQL (Postgres/Oracle), Docker, Kubernetes, CI/CD.
Cloud (AWS/GCP) and mentoring experience preferred.
```

### Golden resume guidelines

* Explicit Java + Spring Boot bullets
* One measurable impact per role
* One cloud reference
* One leadership / mentoring bullet

---

## 7) End-to-End Guest Smoke Script

```bash
set -euo pipefail
mkdir -p out

# Upload
UPLOAD_JSON=$(curl -sS -X POST "$API_BASE/api/v1/documents" \
  -H "X-Guest-Id: $GUEST_ID" \
  -F "file=@./fixtures/golden_resume.pdf" \
  -F "filename=golden_resume.pdf")

echo "$UPLOAD_JSON" | jq .
DOC_ID=$(echo "$UPLOAD_JSON" | jq -r '.document_id')

# Analyze
ANALYZE_JSON=$(curl -sS -X POST "$API_BASE/api/v1/documents/$DOC_ID/analyze" \
  -H "X-Guest-Id: $GUEST_ID" \
  -H "Content-Type: application/json" \
  -d @<(jq -n --rawfile jd ./fixtures/golden_job_description.txt '{
    job_title: "Senior Backend Engineer (Java/Spring)",
    job_description: $jd,
    options: { output_format: "docx", ats_safe: true }
  }'))

ANALYSIS_ID=$(echo "$ANALYZE_JSON" | jq -r '.analysis_id')

# Poll
for i in {1..30}; do
  STATUS=$(curl -sS "$API_BASE/api/v1/documents/$DOC_ID/analyses/$ANALYSIS_ID" \
    -H "X-Guest-Id: $GUEST_ID" | jq -r '.status')
  echo "status=$STATUS"
  [[ "$STATUS" == "succeeded" ]] && break
  [[ "$STATUS" == "failed" ]] && exit 1
  sleep 2
done

# Fetch results
curl -sS "$API_BASE/api/v1/documents/$DOC_ID/analyses/$ANALYSIS_ID/result" \
  -H "X-Guest-Id: $GUEST_ID" \
  -o out/result.json

# Download docx
curl -L -sS "$API_BASE/api/v1/documents/$DOC_ID/analyses/$ANALYSIS_ID/download" \
  -H "X-Guest-Id: $GUEST_ID" \
  -o out/generated.docx
```

---

## 8) Notes from Our Earlier Tests

* `X-Guest-Id` is mandatory for guest flows
* Guest identity must be **stable per browser/session**
* Analysis failures should still be queryable via status API
* DB errors (e.g. `SQLSTATE 42P08`) often indicate missing explicit casts for nullable params

---

If you want, next I can:

* Convert this into a **Postman collection**
* Add **automated golden tests** (Go / Jest)
* Add a **DOCX XML validator** script for ATS rules
