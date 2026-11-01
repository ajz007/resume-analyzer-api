# Durable purchase recovery

## Customer behavior

Repeated checkout requests for the same user/report use one durable purchase and one provider order. A ready checkout keeps the originally saved currency and amount. Selecting another currency does not create a second purchase. A refunded purchase requires support before this report can be purchased again.

The owner-scoped `GET /api/v1/payments/purchases/:analysisId` endpoint returns `none`, `pending`, `ready` (with the saved checkout order), `paid` or `refunded`. It never creates a purchase or provider order. It can attach an existing provider order and grant access for a verified captured payment. The browser calls it on load and on status checks, so recovery does not depend on session storage, the original checkout callback or the original device.

New checkout reserves a purchase in PostgreSQL before calling Razorpay. A unique user/report key elects one creator across Lambda instances. Only that creator makes the external order POST. After any uncertainty, later requests query Razorpay by the purchase UUID receipt. Exact receipt, currency, amount and order binding must match. Ambiguous results require support. Captured payment details must also match before access is granted. A refunded late extra attempt does not hide the valid captured payment on the same order.

Razorpay binds a single successful payment to an order and disallows further payments once paid. See [order states](https://razorpay.com/docs/payments/orders) and [receipt-filtered order lookup with expanded payments](https://razorpay.com/docs/api/orders/fetch-all).

## Release order

1. Deploy API migration 0026 and API code first. It adds payment_purchases without changing prices, quotas or existing payment/unlock records.
2. Pause new checkout while transitioning from the old creator implementation, drain in-flight old requests, then resume once the new API is active. Old and new order creators running simultaneously cannot provide the new guarantee; the old implementation does not respect reservations.
3. Deploy the companion UI after the recovery endpoint is available. The previous UI remains compatible with the API. The new UI blocks checkout if recovery is unavailable, including against an older API.
4. Validate delayed capture, provider interruption, callback loss, refresh and checkout dismissal in Razorpay test mode before wider promotion.

Migration backfill prefers an existing paid order, otherwise the earliest created/failed/refunded order. All historical records are retained. Multiple historical orders for one report set legacy_ambiguous and block further checkout until support reconciliation; existing report_unlocks still provide paid access. The repository also adopts orders saved between the migration and API switch. Already issued legacy orders outside this UI cannot be cancelled by this change.

Application rollback must preserve the reservation table and pause checkout if rolling back to the old creator code. Migration Down deliberately refuses to delete the table, since deleting an uncertain reservation can lead to another collection attempt.

## Operational recovery and limits

A crash after reservation but before the external POST, a definitive provider rejection, or a timeout with no visible provider order leaves the purchase pending. This is intentional: an empty lookup is not proof that a timed-out request cannot still complete. There is no automatic expiry, deletion or blind retry. Support must verify provider records and that no creation request remains in flight before considering a new purchase. This PR does not ship a reset button or automated reset job.

For support triage, use the owner/report purchase UUID as the provider receipt and compare all historical local order/payment records with the provider dashboard. Determine whether the purchase is unresolved, paid, refunded or a genuine pre-provider failure. Do not delete reservations simply to clear an error. Ambiguous legacy duplicates and changing currency require deliberate support resolution, not another checkout click.

The existing signed capture webhooks and verified checkout callbacks remain available. Refund event durability/reordering, a full partial-refund entitlement policy, durable operator tooling, and credit recovery for failed resume reviews remain separate launch gates. This increment conservatively blocks a purchase showing a refund during recovery; it does not claim to solve all refund accounting.

## Validation

- Full backend test suite and payments vet checks passed locally with the temporary PostgreSQL integration database enabled.
- Five PostgreSQL integration tests cover concurrent service instances, recovery after local save failure, immutable order attachment, migration preservation/legacy ambiguity, and adoption of an order saved after migration. Each test uses and removes an isolated schema on a disposable loopback-only PostgreSQL instance.
- Service/provider/HTTP tests cover lost order responses, no blind retry, captured-payment recovery without checkout proof, owner/guest access checks, receipt filtering, duplicate results, provider failures, original pricing, authorization/refund states and late-attempt auto-refunds.
- Run integration tests using PAYMENTS_TEST_DATABASE_URL pointing to a disposable local PostgreSQL database, then `go test ./internal/payments -run TestPG -count=1`. Tests skip without this variable and refuse a non-loopback host.
- Browser tests simulate the provider and validate interruption recovery, pending confirmation, missing tab state, saved quotes, dismissal/resume, refund blocking and no automatic extra order creation, alongside 36 responsive route checks.
- No live provider call, charge, deployment, migration or production write was performed by this work. Local migrations ran only against the disposable test database.
