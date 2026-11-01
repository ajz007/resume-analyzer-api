# Failed-review allowance recovery

## Behavior

New analyses reserve one monthly allowance unit using their analysis UUID. Repeated reservation of that same review cannot consume another unit. A delivered review commits the unit; a review persisted as failed releases it once. Explicit retry after a failed review creates a new analysis with its own reservation, after the failed review's unit has been recovered.

Queue-send failures are persisted as failed and settled. If no queue is configured, no analysis or reservation is created. If quota reservation loses a concurrent limit race, the unqueued analysis is failed without consuming another unit.

The PostgreSQL usage reader reconciles reserved reviews against their saved terminal status in the same transaction as the usage counter update. A worker interruption after saving failure but before settlement therefore recovers on the next usage read or quota check. The counter and ledger state transition commit together, so simultaneous readers cannot release the same unit twice. Caller-supplied failure claims alone cannot cause a refund.

Each reservation records its original billing user, reset boundary and usage epoch. A late failure from a prior month, or before a dev-only allowance reset, cannot subtract from a new window's usage. If a guest claims a review after submission, settlement still applies to the original allowance; balances are not transferred by this change.

Analysis terminal states cannot be reversed through the status/result repository methods. A late failure cannot turn a completed review into a refunded one, and a late result cannot overwrite a failed review after its allowance was returned. Completed review-session edits continue through their existing update path.

The UI refreshes server usage after technical and parse failures, as well as quota errors, instead of adjusting the balance optimistically. A browser timeout does not establish that a server job failed, so it does not itself return allowance. Account/review resets still suppress stale callbacks.

## Commercial scope

This is monthly analysis allowance accounting, not a payment refund or a purchased-credit wallet. Existing guest/free limits, prices, report unlocks and payments are unchanged. Clarification-answer attempts and optional resume generation are not converted to this ledger. Purchased packs and their refund allocation remain future work.

No legacy charge is inferred from an analysis row: the previous counter could increment independently of job creation, so historical rows cannot establish which reviews consumed a unit. Migration 0027 preserves existing counters and begins recording new reservations. Historical failed reviews need explicit reconciliation rather than an automatic guessed refund.

## Release

1. Deploy migration 0027 before updated API and worker code. It adds usage.usage_epoch and analysis_usage_reservations without rewriting historical balances.
2. Start with no active analyses, avoid new submissions during rollout, and ensure both API and worker updates have completed before resuming. Old workers do not enforce the new final-state guards; mixed old/new workers cannot provide the new guarantee.
3. Deploy the small UI refresh change afterward. No new client endpoint or response field is required; the UI remains compatible with the previous API, which simply cannot restore allowance automatically.
4. Validate a controlled technical failure and explicit retry in the intended environment. No live LLM call or production balance reset was performed in local validation.

Keep reservation data and the usage epoch on application rollback. Migration Down deliberately refuses to remove them. A code rollback disables the new recovery behavior until the updated code is restored.

## Known remaining reliability gates

This increment handles persisted terminal failures and returned queue errors. A process crash between reservation and queue dispatch can still leave a queued job with reserved allowance; a worker killed before persisting failure can leave processing state. Queue-send success can also be uncertain. These require a durable dispatch outbox, recovery policy and worker claims/fencing before broad launch. Do not blindly return allowance or start another provider call based solely on a timeout.

Duplicate worker deliveries can still incur duplicate provider calls; terminal guards protect delivered outcomes and allowance accounting but are not a worker lease. Daily cost limits and the per-operation provider-cost ledger remain separate launch work. Database outages may delay recovery until the saved failure and usage records can be read again.

## Validation

- Full Go suite passed with real PostgreSQL integration tests enabled; vet checks for analyses and usage passed.
- Eight PostgreSQL test groups apply all migrations in isolated schemas on a disposable loopback-only database. They cover interrupted settlement, concurrent first-use quota checks, same-review duplicate reservations/settlements, month/reset separation, immutable terminal outcomes, original billing ownership, service retry/completed reuse and returned queue failures.
- Memory/service regression tests cover idempotency, reset isolation, late failure after completion, both submission paths and absent queue configuration.
- Run database tests with USAGE_TEST_DATABASE_URL set to a disposable local PostgreSQL database and `go test ./internal/usage -run TestPG -count=1`. They skip without that variable and reject non-loopback hosts.
- UI unit tests include authoritative usage refresh for technical/parse failures and suppression of a stale failure after reset. Production UI build passed.
- No provider call, charge, production migration or production balance change occurred. All database tests used temporary local schemas.
