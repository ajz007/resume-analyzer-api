# Payment confirmation increment

## Behavior

Checkout verification now performs a read-only Razorpay payment lookup after verifying the checkout signature and order ownership. It grants access only for a matching payment ID, order ID, amount and currency in captured state with zero amount refunded. It does not capture, charge or refund money.

`POST /api/v1/payments/verify` retains its successful response. Additional failures:

| HTTP | Code | Recovery |
| --- | --- | --- |
| 409 | payment_pending | Recheck the existing payment. Do not initiate another purchase. |
| 503 | payment_verification_unavailable | Retry verification later; signed capture webhooks can also confirm payment. |
| 400 | payment_mismatch | Do not unlock; investigate the order/payment association. |

The existing UI preserves the checkout proof in memory and a pending marker in the tab, then refetches owner-scoped access when checking status. Proof recovery after refresh and cross-device reconciliation are still planned work.

An enabled paywall no longer becomes disabled when a Razorpay credential is missing. Unpaid reports remain locked and previously paid unlocks remain readable through the repository. Creating a new order returns 503 when the provider is unavailable. An explicit PAYWALL_ENABLED=false retains its existing behavior.

## Validation

- Full `go test ./...` passed locally.
- `go vet ./internal/payments ./internal/bootstrap` and `git diff --check` passed.
- Regression tests cover authorized/failed/refunded payments, partial refunds, mismatched payment/order/amount/currency, provider failure, retry after capture, provider HTTP authentication and decoding, endpoint recovery codes, explicit paywall disable, missing credentials and preservation of existing paid access.
- Race-detector execution is unavailable in this environment because no C compiler is installed; no race-detector result is claimed.
- No live Razorpay request, charge, production write or deployment was performed. Provider tests use a local HTTP server.

## Release and limitations

No database migration, new environment setting, price change or allowance change is required. Verify provider read credentials and capture settings in the intended environment before deployment. Test authorized-to-captured and delayed-webhook flows in Razorpay test mode as a release gate; these have only been simulated locally.

This change adds a provider request to callback verification. If the provider cannot be reached, the existing report remains locked until verification or a signed capture webhook succeeds; it does not roll back existing paid access. Refund webhook ordering, partial-refund entitlement policy, durable pending-order recovery, duplicate purchase prevention and usage-credit recovery remain unresolved launch gates in plan.md.

Reference: https://razorpay.com/docs/api/payments/fetch-with-id
