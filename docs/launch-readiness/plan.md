# Rethink Resume: paid launch plan

Updated 1 November 2026. This is a staged implementation plan, not a declaration of launch readiness. No production prices, quotas or payment settings are changed by this plan.

## Product and commercial decisions

Sell a completed, truthful review for a target job. Reopening it, saving choices and downloading it should not cost another credit. Define the included clarification allowance before checkout. Preserve uploaded source resumes for reuse and label reviews by company and role.

Start with single purchases and three-review packs. Proposed launch experiments: India INR299 single / INR799 pack; international USD7 single / USD19 pack. Compare single-review demand with INR399 / USD9 after collecting initial evidence. These are test hypotheses, not validated profitable prices. Keep existing prices until accounting, cost measurements and product packaging are ready. Do not promise unlimited AI use or launch a subscription yet.

Support English-speaking international users with the same core experience if the existing payment provider approves the account for international collection. Confirm supported currencies, settlement, refunds, tax treatment and customer-facing policies before advertising international checkout. A new payment provider is not automatically required.

## 1. Make payments and usage trustworthy — required before wider promotion

### First increment: payment confirmation and paywall configuration

Implemented on `codex/payment-resilience`, pending review and deployment:

- Confirm checkout signatures, ownership, provider payment ID, order ID, amount, currency and captured status before granting new access. Refunded payments must not create new access through checkout.
- An authorized payment returns a recoverable pending result; retry verification of the existing payment rather than collecting payment again.
- Provider errors leave local payment and access unchanged.
- An enabled paywall stays enabled when provider credentials are incomplete. Existing paid access remains readable; new checkout is unavailable.
- Automated regression coverage uses a local fake provider and does not charge money.

This increment does NOT solve duplicate order creation, refund ordering or credit accounting. It adds a provider read to checkout verification; signed capture webhooks remain a recovery path.

### Next payment increment

- Reserve one active purchase per user/report in the database before creating an external order; concurrent requests reuse the purchase.
- Reconcile uncertain provider responses against the reserved purchase. Never blindly retry an order creation after an ambiguous timeout. Specify what happens when the provider created an order but the local save failed.
- Persist authenticated webhook events and refund facts, including events arriving before payment linkage. Make retries idempotent and establish an explicit partial-refund policy.
- Reconcile pending purchases and expose a safe recovery state in the UI across refreshes and devices.
- Acceptance: concurrent checkout, response loss, database failure, reordered capture/refund, duplicate webhook and provider outage tests demonstrate at most one entitlement per intended purchase and no revoked payment reactivation. Prove database behavior with PostgreSQL integration tests.

### Usage and purchased-review accounting

- Create a durable credit ledger with separate free allowances and purchased credits. Current monthly usage counters are not a purchased wallet.
- Reserve a credit once per review; commit it on successful delivery. Release it exactly once on technical failure. A user-requested new job review is a new operation; reopening a result is not.
- Tie reservation, job creation and queue dispatch through a durable outbox so a queue failure cannot strand a charged review.
- Add a conditional worker claim/lease so concurrent queue deliveries cannot generate the same report twice. Define recovery after a worker crashes during an uncertain provider call.
- Preserve current balances and historical paid unlocks during migration. Do not reduce existing allowances without an explicit rollout decision.
- Acceptance: parallel submissions, duplicate delivery, worker crash, exhausted quota, queue failure, first-user creation race and calendar rollover are tested against durable storage.

## 2. Bound and measure AI spending — required before changing the free offer

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
