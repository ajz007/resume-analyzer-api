# Queued and interrupted analysis recovery

## Result

Production admission now saves the analysis, its monthly allowance reservation and an SQS dispatch record in one PostgreSQL transaction. Losing the HTTP process before dispatch no longer leaves a charged review without a durable delivery record. A quota race or dispatch-record insertion failure rolls back the entire admission. Bootstrap explicitly wires this mode for the production PostgreSQL repository; memory-only development remains a non-durable fallback.

A temporary or ambiguous queue-send error keeps the same accepted review queued. This supersedes the earlier immediate queue-error failure behavior in PR #31 for the durable PostgreSQL path. Clients continue polling/reopening the same analysis rather than submitting another charged review. Memory/legacy repository configurations retain the earlier fallback behavior.

Dispatch uses a one-minute lease with a unique token. A sender never holds a database transaction while calling SQS. Sending is limited to ten seconds, and late acknowledgements cannot clear another sender's lease. A successful send is eligible for resend after five minutes if no worker claimed the review; a failed send becomes eligible after one minute. There are at most eight dispatch attempts per review, including interrupted attempts. Once the queued review is an hour old it becomes a retryable failure instead of waiting forever. Its established allowance reservation is released once.

## One execution claim per analysis

A production worker atomically changes queued to processing only when started_at is empty and an established allowance reservation exists. Duplicate deliveries acknowledge without running the analysis again. A processing attempt is never automatically reclaimed; resetting its status alone does not make it executable again. Existing provider repair/transient retries inside the one claimed attempt remain unchanged; this is not a promise of one provider HTTP request or one provider bill.

The analysis context has a ten-minute execution budget. If a processing record is still present after twenty minutes, recovery persists an ANALYSIS_INTERRUPTED failure and settles the established allowance reservation. A late result cannot reverse that terminal outcome. An explicit user retry creates a different analysis and reservation. Recovery never automatically starts another provider attempt with an unknown prior outcome.

A remote provider may still bill for an abandoned/timed-out call. This change protects duplicate execution, customer outcomes and allowance accounting; per-operation cost controls remain separate work.

## Recovery entry points

- Opening/polling an owner-scoped analysis result performs recovery after ownership is checked. A foreign owner cannot dispatch or fail another user's review.
- Reusing a saved analysis submission can recover its pending dispatch without a new allowance charge.
- The existing worker accepts a trusted Lambda invocation payload of `{"type":"analysis_recovery"}`. This runs a bounded sweep, not inline AI work. It checks up to 25 candidates with a thirty-second sweep budget.
- The sweep retries unfinished settlement of known completed/failed reviews, including recovery after a crash between saving status and settling usage. Deleted queued/processing analyses are failed and settled without running. Missing/deleted queue messages are acknowledged without execution.

Unattended recovery requires a deployed/enabled schedule. Code merging alone does not create that schedule. Opening a saved review remains an independent recovery path.

## Existing records

Migration 0028 adopts queued analyses with an established reserved usage record from PR #31. Recovery also adopts such records created during a migration/code transition. Old queued records with no usage reservation are not automatically dispatched, because their charge/admission history cannot be established. They can expire into a failed state without guessing a historical allowance refund. Historical successful reports, purchased report access, prices and allowance limits remain unchanged.

## Deployment and activation

1. Begin with no active analyses and avoid new submissions during rollout. Apply migration 0028, then deploy both HTTP API and worker binaries before resuming. Old workers do not honor the new execution claim; mixed versions cannot provide the duplicate-execution guarantee.
2. Verify the worker's configured timeout leaves room for the ten-minute attempt budget and cleanup. The context budget also respects an earlier caller deadline. Keep existing SQS partial-batch handling enabled.
3. Review the optional CloudFormation template at infra/analysis-recovery.json. It targets the existing worker; it does not create a third Lambda or modify CI/deploy workflows.
4. Create a CloudFormation change set with the existing worker Lambda ARN, worker execution role name, and analysis queue ARN. Review the exact resources before applying it. The template adds only a scoped SendMessage policy on that queue, a rule and rule/account-scoped Lambda invoke permission. It must be integrated with existing infrastructure ownership; AWS-side template validation/change-set validation has not been performed locally.
5. Initially keep ScheduleState=DISABLED. Validate the worker event with the JSON payload above in a controlled environment. The production worker needs permission to send recovery messages, not only receive/delete SQS messages.
6. Enable the reviewed rule after validation. The fixed cadence is every thirty minutes, with up to two EventBridge delivery retries. Idle sweeps and database wakeups still have usage/cost implications; no zero-cost claim is made. Review any alarms for repeated recovery failures and existing SQS dead-letter handling.

Do not clear started_at, reset a processing analysis to queued, delete reservations/outbox records, or bypass terminal guards to retry uncertain work. Use the explicit retry flow, which creates a new analysis after the prior attempt is failed.

Application rollback must retain dispatch/reservation data and pause submissions while changing worker versions. Migration Down deliberately refuses to delete dispatch history. Disable the new schedule if rolling the worker back to a version that cannot decode its event. Restoring an old worker also removes the execution-claim guarantee.

## Validation

- Full Go suite passed with PostgreSQL job/usage tests enabled; focused checks passed again after the final claim guard. Vet and whitespace checks passed.
- Fourteen PostgreSQL recovery test groups use isolated schemas on a disposable loopback-only server. They cover atomic rollback, crash before queue send, owner isolation, concurrent dispatcher leases with a two-connection pool, lost SQS responses, concurrent processing claims, abandoned processing, queued deadlines, stale acknowledgement tokens, concurrent admission ceilings, legacy migration/adoption, untracked historical allowance preservation, deletion, bounded retries, interrupted settlement and preventing a second processing claim.
- A controlled fake-provider test holds one worker inside its provider call while a duplicate delivery arrives; only the original worker calls the provider. It verifies propagation of the execution budget and failed-attempt settlement.
- Worker tests cover SQS/scheduled event routing, malformed/mixed events, no inline SQS processing from recovery events, and compatibility between the schedule template's payload and handler. The schedule defaults to disabled.
- Enable database tests with JOBS_TEST_DATABASE_URL and USAGE_TEST_DATABASE_URL pointing to a disposable local PostgreSQL database. Tests skip without their variables and reject non-loopback hosts.
- No live provider call, SQS send, CloudFormation change, production migration, payment or deployment occurred. AWS schedule activation and provider-specific deployment validation remain release actions.

AWS references: [SQS/Lambda duplicate-delivery behavior](https://docs.aws.amazon.com/lambda/latest/dg/with-sqs.html), [Lambda timeouts](https://docs.aws.amazon.com/lambda/latest/dg/configuration-timeout.html), [EventBridge rule target Input and retry configuration](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-properties-events-rule-target.html), [scoped Lambda invoke permission](https://docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-lambda-permission.html).
