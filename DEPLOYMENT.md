# Deployment (GitHub Actions)

Both repos deploy from `.github/workflows/deploy.yml`:

| Repo | On pull request | On push to `main` / manual run |
|---|---|---|
| resume-analyzer-api | `go vet`, `go test` | test → **approval** → `cmd/migrate` → deploy HTTP + worker Lambdas → health check |
| resume-analyzer-ui | `vitest`, `vite build` | test → **approval** → production build → S3 sync → CloudFront invalidation |

The approval comes from the GitHub `production` environment. AWS access uses
GitHub OIDC, so there are no long-lived AWS keys in GitHub.

Migrations run **before** the new Lambda code ships. Keep migrations additive
(new tables/columns, nullable or defaulted) so the currently deployed code keeps
working until the new code is live. To redeploy without migrating, run the API
workflow manually with **Skip database migrations** checked.

## One-time AWS setup

### 1. GitHub OIDC provider

IAM → Identity providers → Add provider:

- Provider type: OpenID Connect
- Provider URL: `https://token.actions.githubusercontent.com`
- Audience: `sts.amazonaws.com`

(Skip if the account already has it.)

### 2. Deploy role

Create an IAM role, e.g. `github-deploy-resume-analyzer`, with this trust policy.
It only trusts jobs that run in the `production` environment of the two repos:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::<ACCOUNT_ID>:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
          "token.actions.githubusercontent.com:sub": [
            "repo:ajz007/resume-analyzer-api:environment:production",
            "repo:ajz007/resume-analyzer-ui:environment:production"
          ]
        }
      }
    }
  ]
}
```

Attach this inline permission policy (replace the placeholders):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DeployLambdas",
      "Effect": "Allow",
      "Action": [
        "lambda:GetFunctionConfiguration",
        "lambda:GetFunction",
        "lambda:UpdateFunctionCode"
      ],
      "Resource": [
        "arn:aws:lambda:<REGION>:<ACCOUNT_ID>:function:<HTTP_FUNCTION_NAME>",
        "arn:aws:lambda:<REGION>:<ACCOUNT_ID>:function:<WORKER_FUNCTION_NAME>"
      ]
    },
    {
      "Sid": "SyncUiBucket",
      "Effect": "Allow",
      "Action": ["s3:ListBucket"],
      "Resource": "arn:aws:s3:::ra-prod-ui-us-east-1-ajz007"
    },
    {
      "Sid": "WriteUiObjects",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::ra-prod-ui-us-east-1-ajz007/*"
    },
    {
      "Sid": "InvalidateCdn",
      "Effect": "Allow",
      "Action": ["cloudfront:CreateInvalidation", "cloudfront:GetInvalidation"],
      "Resource": "arn:aws:cloudfront::<ACCOUNT_ID>:distribution/<CLOUDFRONT_DISTRIBUTION_ID>"
    }
  ]
}
```

If the UI bucket uses SSE-KMS with a customer-managed key, also allow
`kms:GenerateDataKey` and `kms:Decrypt` on that key.

## One-time GitHub setup (each repo)

### `production` environment

Settings → Environments → New environment → `production`:

- Required reviewers: yourself (this is the approval gate)
- Deployment branches: `main` only

### Repository variables

Settings → Secrets and variables → Actions → **Variables**:

| Variable | API repo | UI repo | Example |
|---|---|---|---|
| `AWS_REGION` | ✓ | ✓ | `us-east-1` |
| `AWS_ROLE_ARN` | ✓ | ✓ | `arn:aws:iam::<ACCOUNT_ID>:role/github-deploy-resume-analyzer` |
| `LAMBDA_HTTP_FUNCTION` | ✓ | | HTTP Lambda name |
| `LAMBDA_WORKER_FUNCTION` | ✓ (optional) | | worker Lambda name; leave unset to skip |
| `API_HEALTH_URL` | ✓ (optional) | | `https://api.rethinkresume.com/api/v1/health` |
| `UI_BUCKET` | | ✓ | `ra-prod-ui-us-east-1-ajz007` |
| `CLOUDFRONT_DISTRIBUTION_ID` | | ✓ | `E1234ABCD...` |
| `VITE_API_BASE_URL` | | ✓ | `https://api.rethinkresume.com/api/v1` |

### Secrets

API repo → Settings → Environments → `production` → **Environment secrets**:

- `DATABASE_URL`: the production Postgres URL (`...?sslmode=require`)

Lambda runtime configuration (env vars such as `OPENAI_API_KEY`, `JWT_SECRET`,
`RAZORPAY_*`) stays on the Lambda functions. The workflow only replaces code, so
existing function configuration is untouched.

## Notes

- The workflow reads each function's configured architecture (`arm64` /
  `x86_64`) and builds the `bootstrap` binary to match. Functions must use the
  `provided.al2` / `provided.al2023` runtime, as they do today.
- Deploys never overlap (`concurrency` group); pull-request runs cancel older
  runs on the same branch.
- Rollback: re-run the workflow for an older commit (Actions → run → Re-run), or
  revert the commit on `main`. Migrations are not rolled back automatically.
