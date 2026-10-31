# Application workspace foundation

The workspace adds reusable uploaded sources and owner-scoped application headings without changing generated reports, submitted evidence, score calculation or payment policy.

## API

All routes use the existing authentication middleware and return `Cache-Control: no-store`.

- `GET /api/v1/workspace/primary-resume` returns the owner's explicit primary document, or their latest upload with `isPrimary:false` when no accessible primary is set. The response uses the existing upload fields plus `isPrimary`.
- `PUT /api/v1/workspace/primary-resume` accepts `{documentId}`. Requires sign-in and verifies source ownership. Selecting a primary changes a preference, not the upload or existing analyses.
- `GET /api/v1/applications?q=&limit=20&offset=0` lists the signed-in owner's reviews. Search matches confirmed company/role labels, source filename and a short legacy posting excerpt. Search is literal, including `%` and `_`; it runs before pagination. Limit is 1–50; search is at most 120 characters.
- `GET /api/v1/applications/:id` returns owner-only context: analysis/document IDs, source filename, original job excerpt, mode, status, creation date and labels. It includes no report body, scores or private answers.
- `PUT /api/v1/applications/:id` accepts `{companyName,targetRole}`. Each label is optional and at most 120 characters. This changes display metadata only. A guest can label their own review; labels follow the authoritative analysis owner when the existing guest-claim flow reassigns that analysis.

Unknown and foreign analysis IDs return the same 404. Guest application listing and primary mutation require sign-in. The public share endpoint does not load application context.

## Persistence and compatibility

Migration `0025_application_workspace.sql` adds `document_preferences` and `application_labels`. Existing document and analysis rows are unchanged. Labels are keyed by analysis ID so guest claims do not require a second ownership migration. Preference selection is an atomic owner-checked upsert. Metadata is removed if its parent is hard-deleted; normal source changes do not alter prior reviews.

Apply the additive migration and release the API before the companion UI is released. No production migration or settings change was performed while preparing this branch.

The review source remains its existing immutable document ID. This first increment does not introduce named version families, primary selection for structured builder drafts, automatic extraction-to-builder conversion, or promotion of answer facts into a reusable resume.

The existing analysis reuse rules are unchanged. Repeating an identical source/job may reopen the existing cached analysis. Display-label changes do not trigger generation or re-scoring. A different job description produces the existing independent review behavior.

## Validation

Full Go tests and `go vet ./...` passed. New tests cover primary ownership and fallback, later uploads preserving an explicit primary, private context authorization, label validation, search before pagination, literal search escaping and owner checks in the PostgreSQL upsert. No model calls were made.

PostgreSQL queries are checked with SQL mocks; the migration has not been applied to a live database in this work. A staging database and complete browser acceptance pass remain required before production acceptance.
