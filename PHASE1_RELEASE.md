# Phase 1 Release — Resume Analyzer

## 1) Overview
Phase 1 delivers a minimal, reliable upload → analyze → results flow with async job status and actionable failure states. It targets quick feedback loops for early adopters and demo environments while keeping scope focused on analysis only.

This release is intended for guest users, early adopters, and demo usage where fast iteration and clear status visibility matter more than production auth or resume rewriting. Analysis is executed asynchronously via background workers and exposed through a polling API.

## 2) Included in Phase 1
- Resume upload (PDF/DOC/DOCX ≤5MB)
- Job description analysis (≥300 chars enforced)
- Async analysis with polling
- Actionable results (completed/failed with errorMessage)
- Guest isolation enforced
- Deterministic post-processing (temperature=0 where supported + list normalization)

## 3) Explicitly Excluded (Phase 2)
- Resume rewriting
- DOCX generation / apply flows
- Generated resumes download
- Production authentication & billing
- Advanced AI reasoning guarantees

## 4) How to Run (local)

Required environment variables (minimum):
- `OPENAI_API_KEY` (when `LLM_PROVIDER=openai`)
- `LLM_PROVIDER` (default: `openai`)
- `LLM_MODEL` (required for OpenAI)
- `OBJECT_STORE` (default: `local`)
- `LOCAL_STORE_DIR` (when `OBJECT_STORE=local`)
- `DATABASE_URL` (required when `ENV=production`)

Start API:
```bash
go run ./cmd/api
```

Phase 1 test gate:
```bash
go test ./...
```

## 5) Smoke Test
Run the guest smoke test flow referenced in `PHASE1_DOD.md` (Release checklist → “Guest smoke test”). Do not use the Phase 2 apply/DOCX steps.

## 6) Known Limitations
- LLM output is best-effort deterministic (especially GPT-5), even with temperature=0.
- No automatic recovery for stuck jobs.
- Guest history requires login.

## 7) Phase 1 Freeze Statement
Phase 1 scope is frozen. Any new features or changes beyond this document’s “Included in Phase 1” must be filed and scheduled in the Phase 2 backlog.
