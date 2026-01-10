# Resume Analyzer API

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
