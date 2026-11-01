# Rethink Resume: paid launch plan

Updated 1 November 2026. This is a staged implementation plan, not a declaration of launch readiness. No production prices, quotas or payment settings are changed by this plan.

## Product and commercial decisions

Sell a completed, truthful review for a target job. Reopening it, saving choices and downloading it should not cost another credit. Define the included clarification allowance before checkout. Preserve uploaded source resumes for reuse and label reviews by company and role.

Start with single purchases and three-review packs. Proposed launch experiments: India INR299 single / INR799 pack; international USD7 single / USD19 pack. Compare single-review demand with INR399 / USD9 after collecting initial evidence. These are test hypotheses, not validated profitable prices. Keep existing prices until accounting, cost measurements and product packaging are ready. Do not promise unlimited AI use or launch a subscription yet.

Support English-speaking international users with the same core experience if the existing payment provider approves the account for international collection. Confirm supported currencies, settlement, refunds, tax treatment and customer-facing policies before advertising international checkout. A new payment provider is not automatically required.

## 1. Make payments and usage trustworthy — required before wider promotion

### First increment: payment confirmation and paywall configuration

Merged in PR #29:

- Confirm checkout signatures, ownership, provider payment ID, order ID, amount, currency and captured status before granting new access. Refunded payments must not create new access through checkout.
- An authorized payment returns a recoverable pending result; retry verification of the existing payment rather than collecting payment again.
- Provider errors leave local payment and access unchanged.
- An enabled paywall stays enabled when provider credentials are incomplete. Existing paid access remains readable; new checkout is unavailable.
- Automated regression coverage uses a local fake provider and does not charge money.

This increment does NOT solve duplicate order creation, refund ordering or credit accounting. It adds a provider read to checkout verification; signed capture webhooks remain a recovery path.

### Purchase recovery increment

Merged in API PR #30 and UI PR #21:

- Persist a unique purchase before provider creation; repeated requests across processes reuse it.
- Recover uncertain provider responses by exact receipt lookup, never a second blind order POST.
- Recover captured payment/access without browser checkout proof; resume a ready order at its original currency and price.
- Backfill legacy orders and require support for ambiguous historical duplicates. Adopt legacy orders saved between migration and code deployment.
- PostgreSQL integration tests and browser recovery tests passed locally. See purchase-recovery.md for migration, rollout and operational limitations.

### Remaining payment work

- Persist authenticated webhook events and refund facts, including events arriving before payment linkage. Make retries idempotent and establish an explicit partial-refund policy.
- Add reviewed operator tooling for purchases that remain uncertain or contain historical duplicate orders.
- Acceptance: concurrent checkout, response loss, database failure, reordered capture/refund, duplicate webhook and provider outage tests demonstrate at most one entitlement per intended purchase and no revoked payment reactivation. Prove database behavior with PostgreSQL integration tests.

### Failed-review allowance increment

Merged in API PR #31 and UI PR #22:

- Reserve monthly allowance once per analysis UUID, commit delivered reviews, and release persisted failures once.
- Recover an interrupted settlement on the next usage read/quota check; protect month/reset boundaries and the original billing owner.
- Persist returned queue errors as failed and prevent late callbacks from reversing terminal outcomes.
- Refresh the UI's server balance after technical/parse failures. No inferred legacy refunds, price/limit changes or purchased wallet.
- PostgreSQL concurrency/lifecycle tests and service/UI regressions passed. See review-allowance-recovery.md for scope and rollout.

### Analysis job recovery increment

Merged in API PR #32; scheduled resources remain undeployed by the owner's current choice:

- Commit analysis creation, allowance reservation and dispatch together in PostgreSQL; transient queue errors keep the same queued review accepted.
- Lease dispatch attempts and use a single processing claim, preventing duplicate queue deliveries from executing the same analysis again.
- Bound attempts to ten minutes; classify abandoned processing after twenty minutes and queued work after an hour as retryable failures, with established allowance recovery.
- Recover on owner result reads and provide a bounded scheduled sweep. An optional infrastructure template is disabled by default; unattended recovery still requires schedule activation.
- PostgreSQL failure/concurrency tests and worker event-contract tests passed. See analysis-job-recovery.md for migration, activation and rollback instructions.

### Remaining purchased-review accounting and release validation

- Extend the per-review monthly allowance ledger with separate purchased-credit balances. The current monthly counter and new reservation ledger are not a purchased wallet.
- Preserve once-per-review reservation/settlement when adding purchased credits. A user-requested new job review is a new operation; reopening a result is not. Complete recovery for jobs that never reach a persisted terminal state.
- Validate the deployed outbox, worker claim, timeout recovery and schedule under controlled queue interruption before promotion.
- Retain the explicit-retry policy for uncertain provider calls. Extend similar recovery guarantees to other AI job types when they enter the paid offer.
- Preserve current balances and historical paid unlocks during migration. Do not reduce existing allowances without an explicit rollout decision.
- Acceptance: parallel submissions, duplicate delivery, worker crash, exhausted quota, queue failure, first-user creation race and calendar rollover are tested against durable storage.

## 2. Bound and measure AI spending — required before changing the free offer

### Measurement increment

Implemented on `codex/ai-usage-ledger`, pending review/deployment:

- Record each provider HTTP attempt, including retries/repairs and failed/invalid outputs, with internal operation/review identifiers.
- Preserve missing, interrupted and unpriced usage explicitly. Add versioned standard-global GPT-6.1 Sol reference estimates and private read-only coverage/cost reports.
- Use existing PostgreSQL and normal requests only. No new AWS resources, schedules, public cost endpoint, environment variables or paid experiments.
- Measurement is best effort and does not enforce a budget. Check coverage/gap logs before using measured means/P95 to price the offer.
- See ai-usage-measurement.md and ai-usage-reports.sql for deployment, rate assumptions, validation and operator queries.

### Remaining spending controls and validation

- Record provider/model, operation, input/output tokens, latency, retries and known/unknown cost for each review and answer; exclude resume text and personal details from cost logs.
- Set explicit output, input/document and clarification limits on every AI path, including optional generation/tailoring paths. Count repair attempts and retries in the same operation budget.
- Introduce shared usage controls and a daily spend ceiling with an operator stop control. Rotating a guest header must not create an unlimited allowance.
- Measure a representative set of real-length resumes and job descriptions with deliberately bounded spend before choosing final limits. Offline estimates are insufficient for P95 cost.
- Acceptance: token exhaustion, provider errors and the daily ceiling produce understandable recoverable states; no unmetered generation route bypasses the controls.

## 3. Package the offer and implement checkout

Depends on steps 1 and 2.

- Give users a useful, low-cost preview. The current full-generation-then-redaction flow is not a cheap preview; design and price the preview operation explicitly.
- Persist preview results so reloads do not repeat paid provider work. Define whether a full review reuses preview output or is a distinct operation and measure the combined cost.
- Add server-priced single-review and three-review products, durable purchased-credit balances, receipts and payment/refund reconciliation. A three-pack purchase credits exactly three reviews once.
- Show total price, included work, remaining credits and recoverable payment states clearly. Test India and international checkout separately.
- One review includes its stated clarifications, saved edits, reopening and downloads. Explain when a different target job uses a new credit.
- Acceptance: full preview-to-purchase-to-review-to-download journey works with pending captures, declined payment, interrupted sessions and exhausted credits. No prices are published before the corresponding product works.

## 4. Run a small paid launch

Depends on all technical release gates above.

- Validate production with the owner first, including a controlled purchase/refund when the owner authorizes it. Confirm international payment activation and applicable policies before promoting those markets.
- Reach the first 20–50 paying customers through relevant professional communities, referrals and useful job-specific content. Community posts, outreach and ad spend require a separate instruction; this plan does not authorize sending them.
- Measure India and international cohorts separately: preview completion, first useful edit, preview-to-paid conversion, successful delivery/download, support/refund rate, repeat review purchase and acquisition cost.
- Track contribution per delivered review after provider costs, payment fees, refunds, free-preview subsidy and allocated infrastructure; track average and P95 cost separately. Set the acceptable margin and acquisition ceiling using observed figures.
- Keep acquisition small if delivery is unreliable or contribution is negative. Adjust offer, free allowance or price from evidence before buying traffic.

## 5. Expand only after demand and margin are demonstrated

Test the next price against conversion and contribution, then expand the channels that bring paying customers within the measured acquisition ceiling. Defer subscriptions, broad integrations and unrelated feature expansion until repeat usage supports them.

## Operational handoff for each increment

Each implementation has its own reviewable pull request, focused regression tests, migration/recovery instructions where needed, and a clear list of unresolved launch gates. Merging, deploying and changing live commercial settings are separate release actions.

Provider reference for checkout confirmation: https://razorpay.com/docs/api/payments/fetch-with-id
