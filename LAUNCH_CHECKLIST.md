# Paid report unlock: go-live checklist

Steps to turn on Razorpay report unlocks in production. Do them in order; the
paywall stays off until the last step.

## 1. Deploy the code

- Merge the API and UI changes. The API deploy runs migration `0024_payments.sql`
  (see [DEPLOYMENT.md](DEPLOYMENT.md)).
- On the HTTP Lambda, set `ANALYSIS_PROMPT_VERSION=v2_5`. v2_5 uses the v2_4 output
  schema and tells the model to score `atsReadability` and `resumeStructure` from
  the resume alone. The version is picked when an analysis is created, so only the
  HTTP Lambda needs it; the worker uses the version stored on the analysis.
- Check `LLM_MODEL` and `OPENAI_TEMPERATURE`. The OpenAI client silently drops
  `temperature` for models that reject it, so a score-consistency run (step 2) is
  the real test of determinism.

## 2. Score consistency

ATS Readiness should not change when a job description is added. Build a folder
of at least five resume/JD pairs:

```
pairs/
  backend-go/
    resume.pdf      (or resume.docx)
    jd.txt
  data-analyst/
    ...
```

Then run, with the production model:

```bash
OPENAI_API_KEY=... LLM_MODEL=<production model> \
  go run ./cmd/atsconsistency -pairs ./pairs -prompt-version v2_5 -runs 2
```

It prints ATS Readiness with and without the JD for each pair and exits non-zero
if any pair moves by more than 3 points (`-max-delta`).

## 3. Razorpay dashboard

- Complete KYC and activate the account. Razorpay reviews the website, so the
  `/terms`, `/refund`, `/delivery` and `/contact` pages must be live, and the
  support email in the UI's `src/constants/legal.ts` must be a real mailbox.
- Enable International Payments so USD checkout works.
- Add a webhook: URL `https://api.rethinkresume.com/api/v1/payments/webhook`,
  events `payment.captured`, `order.paid`, `refund.processed`. Copy the webhook
  secret.
- If API Gateway lists routes explicitly instead of using a `{proxy+}` route,
  add `/api/v1/payments/{proxy+}`.

## 4. Test mode, end to end

Set test-mode keys on the HTTP Lambda (step 5's variables, with `rzp_test_` keys)
and `PAYWALL_ENABLED=true`, then:

1. Run an analysis as a guest: scores and the top 3 fixes show, then "Sign in to unlock".
2. Sign in, pay with test card `4111 1111 1111 1111`: the full report appears.
3. Reload the page and open the share link: both show the full report.
4. Refund the payment in the dashboard: after the `refund.processed` webhook,
   the report locks again.
5. Pay with an international test card in USD.

## 5. Go live

On the HTTP Lambda, set:

| Variable | Value |
|---|---|
| `RAZORPAY_KEY_ID` | live key id (`rzp_live_...`) |
| `RAZORPAY_KEY_SECRET` | live key secret |
| `RAZORPAY_WEBHOOK_SECRET` | live webhook secret |
| `PRICE_INR_PAISE` | e.g. `49900` (₹499) |
| `PRICE_USD_CENTS` | e.g. `900` ($9) |
| `PAYWALL_ENABLED` | `true`, set this last |

The paywall only turns on when `PAYWALL_ENABLED=true` and all three Razorpay
secrets are present. To turn it off, set `PAYWALL_ENABLED=false`; existing
unlocks are kept.
