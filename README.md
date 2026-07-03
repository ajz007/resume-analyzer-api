# Resume Analyzer API

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
go build -o bootstrap ./cmd/lambda-http

```
## Prompt Testing CLI

Run a resume through extraction + LLM and validate JSON output:

```bash
go run ./cmd/prompttest --resume ./resume.pdf --jd ./job.txt --prompt-version v2_1 --provider openai --model gpt-4o-mini
```

Flags:
- `--resume` (required): Path to resume file (`.pdf` or `.docx`).
- `--jd`: Path to job description file (optional).
- `--prompt-version`: Prompt version string (default `v1`).
- `--out`: Write raw JSON to a file (optional).
- `--provider`: LLM provider (default from env/config).
- `--model`: LLM model (default from env/config).

## Testing

Default tests (Phase 1) run with no tags:
`go test ./...`
Phase 2 tests cover DOCX/apply paths and are behind a tag:
`go test -tags phase2 ./...`
Run both locally when touching DOCX or apply flows.

## Database Migrations

Run schema migrations only through `cmd/migrate`.

Local / any environment with `DATABASE_URL` set:

```bash
DATABASE_URL="postgres://USER:PASSWORD@HOST:5432/DBNAME?sslmode=require" go run ./cmd/migrate
```

Windows PowerShell:

```powershell
$env:DATABASE_URL="postgres://USER:PASSWORD@HOST:5432/DBNAME?sslmode=require"
go run ./cmd/migrate
```

Important for async resume generation:
- migration `0022_resume_generation_jobs.sql` must be applied before `POST /api/v1/resumes/generate` will work
- migration `0023_resume_generation_error_type.sql` adds the `error_type` field used for operator diagnostics
- if it is missing, the API can fail before enqueue with a database error

Quick verification:

```sql
SELECT to_regclass('public.resume_generation_jobs');
```

If that returns `NULL`, the async resume generation table has not been created yet.

## API

### Download generated resume

```bash
curl -L --fail-with-body -o out.docx http://localhost:8080/api/v1/generated-resumes/<id>/download
```

Example browser download (handles JSON errors safely):

```javascript
async function downloadResume(id) {
  const resp = await fetch(`/api/v1/generated-resumes/${id}/download`);
  if (!resp.ok) {
    const payload = await resp.json();
    const message = payload?.error?.message || "Download failed";
    throw new Error(message);
  }
  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "generated_resume.docx";
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
```
