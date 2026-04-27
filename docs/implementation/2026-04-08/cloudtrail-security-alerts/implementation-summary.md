# CloudTrail Security Alerts Implementation Summary

## Implementation Complete

New resource type `aws-cloudtrail-security-alerts` that creates CloudWatch metric filters and alarms for security-relevant CloudTrail events. Covers the full AWS Security Hub/CIS CloudWatch control set (CloudWatch.1-14).

## What Was Implemented

### New Resource Type

Users can define security alerts in their `server.yaml`:

```yaml
resources:
  resources:
    prod:
      resources:
        cloudtrail-security:
          type: aws-cloudtrail-security-alerts
          config:
            logGroupName: aws-cloudtrail-logs-s3-buckets
            logGroupRegion: us-west-2
            email:
              addresses:
                - security@company.com
            alerts:
              rootAccountUsage: true
              unauthorizedApiCalls: true
              consoleLoginWithoutMfa: true
              iamPolicyChanges: true
              cloudTrailTampering: true
              failedConsoleLogins: true
              kmsKeyDeletion: true
              s3BucketPolicyChanges: true
              configChanges: true
              securityGroupChanges: true
              naclChanges: true
              networkGatewayChanges: true
              routeTableChanges: true
              vpcChanges: true
```

### 14 CIS-Aligned Alerts

| CIS Control | Alert | Default Threshold |
|-------------|-------|-------------------|
| CloudWatch.1 | Root account usage | >= 1 / 5min |
| CloudWatch.2 | Unauthorized API calls | >= 5 / 5min |
| CloudWatch.3 | Console login without MFA (success only) | >= 1 / 5min |
| CloudWatch.4 | IAM policy changes | >= 1 / 5min |
| CloudWatch.5 | CloudTrail config changes | >= 1 / 5min |
| CloudWatch.6 | Failed console logins | >= 1 / 5min |
| CloudWatch.7 | KMS key deletion/disable | >= 1 / 5min |
| CloudWatch.8 | S3 bucket policy changes | >= 1 / 5min |
| CloudWatch.9 | AWS Config changes | >= 1 / 5min |
| CloudWatch.10 | Security group changes | >= 1 / 5min |
| CloudWatch.11 | NACL changes | >= 1 / 5min |
| CloudWatch.12 | Network gateway changes | >= 1 / 5min |
| CloudWatch.13 | Route table changes | >= 1 / 5min |
| CloudWatch.14 | VPC changes | >= 1 / 5min |

### Code Changes Summary

| File | Change | Lines |
|------|--------|-------|
| `pkg/clouds/aws/cloudtrail_security_alerts.go` | Config struct, selectors, reader | +41 |
| `pkg/clouds/aws/cloudtrail_security_alerts_test.go` | Config parsing tests | +151 |
| `pkg/clouds/aws/init.go` | Register config reader | +3 |
| `pkg/clouds/pulumi/aws/cloudtrail_security_alerts.go` | Pulumi provisioner (metric filters + alarms) | +310 |
| `pkg/clouds/pulumi/aws/cloudtrail_security_alerts_test.go` | Alert selection + definition tests | +120 |
| `pkg/clouds/pulumi/aws/init.go` | Register provisioner | +2 |

**Total:** 6 files, ~627 lines

## Architecture

Each enabled alert creates:
1. A CloudWatch LogMetricFilter on the specified CloudTrail log group
2. A CloudWatch MetricAlarm (Sum >= threshold in 5 min period)
3. Optional SNS topic + email subscriptions for notifications

### Cross-Region Support

CloudTrail log groups may be in a different region than the main SC deployment. When `logGroupRegion` is specified, a region-specific AWS provider is created with credentials from `AccountConfig`.

### Resource Naming

Resource names are derived from the descriptor name (not hardcoded), supporting multiple instances per stack.

### Notification Channels

- Email: via SNS topic + SNS email subscriptions
- Slack / Discord / Telegram: via SC helpers Lambda (same image used by ECS ALB alerts).
  Each enabled alert gets its own Lambda (deterministic per-alert env vars for
  AlertName/AlertDescription), which pulls the webhook URL from Secrets Manager
  and formats the alarm payload for the target channel. Channels can be combined
  with email for dual delivery.

### Webhook enrichment

The Lambda handler receives only the CloudWatch alarm state-change event —
nothing about *what* caused the alarm to fire. For ECS/ALB metric alarms
that's fine (the alarm name tells you which metric), but for CloudTrail
security alerts the interesting information is in the underlying event
(who called which API, from where). Without that, every Slack message
looks identical and reviewers have to click through to the console.

When the provisioner sets three env vars on the Lambda —
`SIMPLE_CONTAINER_CT_LOG_GROUP_NAME`, `SIMPLE_CONTAINER_CT_LOG_GROUP_REGION`,
`SIMPLE_CONTAINER_CT_FILTER_PATTERN` — the handler does one extra step
before sending:

1. Parse the alarm's state-timestamp to anchor a lookup window
   (`[timestamp - 10min, timestamp + 1min]` to cover the 5-min evaluation
   period + clock skew).
2. `logs:FilterLogEvents` on the CT log group with the same filter pattern
   the metric filter uses — so we get *exactly* the events that drove the
   metric count over threshold.
3. Parse each returned record (CloudTrail JSON) into a `ctEvent`, taking
   only the fields used for display to be resilient to schema drift.
4. Pick the top 5 newest, render as bullet lines, append to the alert
   `Description` so Slack/Discord/Telegram senders all show it without
   changes.

The `Actor()` helper chooses the most-specific human-readable identity
(AssumedRole → `sessionContext.sessionIssuer.userName`, IAMUser →
`userName`, fallback → `arn`, else `"unknown"`). Empty events list
produces an empty string so callers can always append unconditionally.

IAM: the existing `<alert>-xpolicy` managed policy gets an extra
statement granting `logs:FilterLogEvents` scoped to the configured
log-group ARN (not `*`). When the provisioner didn't pass a
`ctLogGroupArn`, the extra statement is omitted so nothing changes for
non-CT alerts.

Enrichment is best-effort: any failure during the lookup is logged at
warn level and swallowed. The alert still goes out with its original
payload — never losing notifications over an enrichment error.

### Trail pre-flight check (log-file validation)

Running security alerts on top of a CloudTrail trail that doesn't have
log-file validation turned on is a compliance gap: events ARE recorded,
but S3 file tampering can't be cryptographically detected after the
fact. Auditors expect the digest files, and a resource type that is
advertised as "SOC2 CC7.1 aligned" should not silently accept that.

When `trailName` is set on the config, the provisioner:

1. Calls `DescribeTrails` against the account/region in the resource's
   auth context with just the one trail name (no shadow trails — we
   only care about the local trail, not replicas in other regions).
2. If the trail is not found → **hard error** (always — not a soft
   warn, because a typo'd trail name is never what the user meant).
3. If the trail exists with `LogFileValidationEnabled == true` → pass,
   info-log and continue.
4. If disabled and `requireLogFileValidation` is unset/true (the
   default) → **hard error** with the exact AWS CLI command to fix it.
5. If disabled and `requireLogFileValidation: false` → **warning** in
   the Pulumi log, continue. For temporary rollout bypass only; the
   check log line is intentionally loud.

The strict/decision logic lives in `evaluateTrailValidation` — a pure
function separate from the AWS-client call (`ensureTrailLogFileValidation`).
That separation is what makes the unit tests meaningful: we drive the
decision with fabricated `cloudtrail.Trail` values instead of mocking
an AWS session.

Back-compat: `trailName` is optional; deployments that don't set it
keep the old behavior (no check, no API call, no new IAM permission
required). To opt in, set `trailName`; to opt into warning-only mode,
also set `requireLogFileValidation: false`.

Required IAM (where the SC CLI runs): `cloudtrail:DescribeTrails` on
`*`. This is broad by AWS IAM standards, but the action is read-only
and the service has no finer-grained resource ARN format for
DescribeTrails.

The helpers image is pushed into an ECR repo namespaced by the SC resource
descriptor name (`<resPrefix>-security-helpers`), so the CloudTrail security
alerts resource can coexist with compute-stack ALB alerts that already use
`sc-cloud-helpers` without URN or ECR-repo collisions. When `logGroupRegion`
is set, the helpers image is pushed into that region so the Lambda can pull
from same-region ECR.

IAM role names (`<resPrefix>-<alert>-execution-role-<pulumi-suffix>`) are
capped via `util.TrimStringMiddle(..., 38, "-")` to stay under AWS's 64-char
IAM role name limit for the longer CIS alert names.

## Compliance Coverage

- SOC 2: CC6.1, CC6.5, CC6.6, CC7.1, CC7.2, CC7.3
- ISO 27001:2022: A.5.16, A.5.18, A.8.2, A.8.5, A.8.15, A.8.16, A.8.20, A.8.22, A.8.24, A.8.32
- NIST 800-53: AU-6, AC-2, AC-6, SI-4
- AWS CIS Benchmark: CloudWatch.1 through CloudWatch.14

## Testing Results

All tests passing (config parsing, alert selection, deterministic ordering, definition validation, unique names).

## Review History

- Self-review: 3 rounds
- OpenAI Codex code review: fixed credential pass-through, naming collisions, MFA filter scope
- OpenAI Codex compliance gap analysis: expanded 7 -> 14 alerts, corrected filter patterns to match CIS reference
- Post-Slack-wiring review: fixed `createSNSTopicForAlerts` tags-arg signature drift;
  fixed IAM role name overflow for CIS alerts with longer names (TrimStringMiddle cap);
  fixed cross-region ECR so the helpers Lambda pulls from same-region ECR
