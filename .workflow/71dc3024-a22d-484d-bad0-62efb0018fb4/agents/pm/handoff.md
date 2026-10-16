[thinking] # PM Handoff — S3: Redact Pulumi secret-envelope values in `procDiagnosticEvent`

## 0. Context restated (from brief)

- **Problem**: `api/pkg/clouds/pulumi/events.go:75` (`procDiagnosticEvent`) forwards `evt.DiagnosticEvent.Message` verbatim at `Info` level. That message is the raw Pulumi gRPC diagnostic payload, which can contain a property Pulumi marked secret (wrapped in its secret envelope — JSON object carrying the signature key `4dabf18193072939515e22adb298388d` = `"sig"`/`sig.Key` with value `1b47061264138c4ac30d75fd1eb44270` = `sig.Secret`, per `pulumi/sdk/go/common/resource/sig`) — yet the plaintext value still leaks because SC's own log line bypasses Pulumi's redaction, which only applies to Pulumi's *own* summaries, not to a diagnostic message SC re-logs itself.
- **Already shipped / explicitly out of scope for this dispatch**: S1 (dedicated `sc-deploy` IAM user, live since 2026-10-07), S2 (key revocation — operator-owned, blocked, do not touch IAM).
- **This dispatch (S3) scope**: ONLY the redaction fix in `procDiagnosticEvent` (or a helper it calls). Not a per-provider fix (e.g. not just AWS's `applyAWSProviderCreds` marking args as secret) — the chokepoint fix must catch any provider's secret-marked diagnostic payload generically.
- **Prior attempts**: 2 prior dispatches of this exact slice died in a developer-character crashloop (`exit status 143`, "claude emitted no output beyond init for 5m0s") before any code was written. Architect's design handoff was clean both times — this looks like infra flake at Developer turn startup, not a content/design problem. Flagging this forward explicitly (see §6).
- **Side-observation only (not in scope)**: this run targets `api`, whose branch protection requires 2 human reviews (blocks SDLC auto-merge). A forge-conductor fix for that is already deployed; Developer/DevOps should note PR number + `mergeable_state` in their handoffs as a side observation, not as work to do.

## 1. Commit + branch identity

No code mutations in this turn — I am the PM (Roadmap/requirements only). No files written to the repo.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | PM turn produces plan only; no repo mutation |

## 3. Requirements (restated, scoped)

**MUST**
1. `procDiagnosticEvent` (or a helper function it calls, still living in `pkg/clouds/pulumi/events.go` or a new file in the same package) must detect when `evt.DiagnosticEvent.Message` contains Pulumi's secret-envelope signature pair (`sig.Key`="4dabf18193072939515e22adb298388d" with value `sig.Secret`="1b47061264138c4ac30d75fd1eb44270" — import `github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig` rather than hardcoding the magic strings, since the constants already exist in the vendored SDK) and mask the wrapped value before the message reaches `p.logger.Info`.
2. The fix must be provider-agnostic: it operates on the diagnostic message text/structure itself, not on a provider's resource args. No provider package (`aws`, `gcp`, `kubernetes`, etc.) should need its own redaction logic for this path.
3. Non-secret diagnostic content (resource URNs, property names, non-secret property values, diff summaries) must remain visible — this is a targeted mask, not a blanket truncation. Existing diagnostic usefulness for debugging deploys must be preserved.
4. Unit test coverage (new `events_test.go` or similar in `pkg/clouds/pulumi/`):
   - a) A synthetic/fixture diagnostic message containing a secret-marked property's value does NOT appear in the string passed to the logger.
   - b) A non-secret property's value in the same message is unaffected (passes through unchanged).
   - c) Direct test(s) of the secret-envelope-signature detection itself (the `4dabf181.../1b470612...` pair), independent of any specific provider's payload shape — this should be testable as a pure function, not only through the full `procDiagnosticEvent` integration path.
   - d) Follow the test-matrix rigor of `pkg/api/logger/redact_test.go`'s sibling convention — i.e. table-driven cases covering both "is redacted" and "is NOT redacted" (negative control), per the fleet norm cited in the brief. Note: `pkg/api/logger/redact.go` already exists in THIS repo (not just forge-conductor) and already wraps `logger.Info`'s `println` with a regex-based `Redact()` — but that existing helper targets PEM/private-key-shaped material (via `privateKeyBlock`/`base64Run` decode-and-sniff), which is a DIFFERENT shape than a plain string/JSON secret wrapped in Pulumi's envelope (e.g. an AWS access key, which isn't base64-PEM-shaped). Architect should decide whether S3's new detection composes with/extends `pkg/api/logger/redact.go`, or is a separate targeted helper in `pkg/clouds/pulumi/` — but do not assume the existing `Redact()` already covers this case; it does not, per the live incident log (AWS key leaked straight through `p.logger.Info` already).
5. No real/live credential values in any commit, test fixture, or handoff — synthetic values only (e.g. `AKIAFAKEEXAMPLE1234`, `fakeSecretXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX`).

**NICE-TO-HAVE (do not block S3 signoff on these)**
6. Extending redaction to the `ResOutputsEvent`/`ResourcePreEvent`/`PolicyEvent` diff-summary paths (`diffSummary`, `procResourceOutputsEvent`, `procResourcePreEvent`) which also log `apitype.PropertyDiff` values — these are a related but separate exposure surface (diff rendering, not raw diagnostic passthrough) and are NOT required by this slice's DoD, which names `procDiagnosticEvent` specifically. Architect may flag as a follow-up roadmap item but should not expand this dispatch's scope.
7. A log-level toggle to suppress diagnostic messages entirely in strict mode — out of scope, no requirement for it in the brief.

**CUT**
8. Any IAM/credential rotation or revocation work (S2) — explicitly operator-owned and blocked; do not touch.
9. Fixing `pkg/clouds/pulumi/aws/provider.go:96-99` (`applyAWSProviderCreds` plain `sdk.String` args) as a standalone fix — the brief explicitly says fixing only that would not close the generic path; it's not what's being asked for in S3 (the chokepoint fix supersedes it for logging purposes; whether AWS's provider args should ALSO be marked `sdk.ToSecret(...)` is a separate, not-requested hardening step).
10. Any UI/CLI surface change, schema regen, or docs-site page — this is an internal logging-safety fix with no user-facing config surface; `welder run generate-schemas` is not implicated.

## 4. Acceptance criteria (for Developer/QA to verify against)

| AC# | Criterion | Verification method |
|---|---|---|
| AC1 | A deploy log produced by the current SC binary contains no credential material for any provider, demonstrated via a synthetic/fixture secret-marked `DiagnosticEvent` (not a live credential) | Unit test asserting masked output for a fixture payload shaped like the real `getResource Invoke RPC finished` message, using a fake AWS-shaped key/secret string |
| AC2 | Redaction lives in `procDiagnosticEvent` or a helper it calls — not scattered per-provider | Code review: no new redaction logic added to `pkg/clouds/pulumi/aws/`, `gcp/`, `kubernetes/`, etc.; the fix is confined to `pkg/clouds/pulumi/events.go` (+ maybe a shared helper in `pkg/clouds/pulumi/` or `pkg/api/logger/`) |
| AC3 | Unit test: secret-marked property's value does not appear in forwarded log message | New test, e.g. `TestProcDiagnosticEvent_RedactsSecretEnvelope` |
| AC4 | Unit test: non-secret property's value is unaffected | New test, e.g. `TestProcDiagnosticEvent_PreservesNonSecretValues` |
| AC5 | Unit test: Pulumi secret-envelope signature detection itself (`4dabf181.../1b470612...`) is independently tested | New test, e.g. `TestDetectsPulumiSecretEnvelope`, ideally against `sig.Key`/`sig.Secret` constants rather than hardcoded strings |
| AC6 | Follows fleet logging/redaction conventions (table-driven, positive+negative cases) per `pkg/api/logger/redact_test.go`-style rigor | Code review of new test file's structure |
| AC7 | No real credential value appears anywhere in the PR's commits/tests/handoffs | Code + handoff review — grep diff for anything resembling a live AWS key (`AKIA[0-9A-Z]{16}`) |

## 5. Handoff guidance for Architect

- **Entry point**: `pkg/clouds/pulumi/events.go:75`, function `procDiagnosticEvent`. Current body:
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
- **Key design question for you**: `evt.DiagnosticEvent.Message` is a plain `string` (from `pulumi/sdk/v3/go/auto/events`), not a structured `resource.PropertyValue`/`PropertyMap` — so "detecting the secret envelope" here means pattern-matching the JSON-ish text representation of Pulumi's envelope (`"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270"` alongside a sibling `"value":"<secret>"` key) as it appears serialized inside the diagnostic string, likely from a gRPC/JSON dump embedded in the message. Decide the matching strategy (regex vs. a lightweight JSON-aware scan for the envelope shape) — favor simplicity and matching what the actual leaked log looked like (plaintext key+secret printed 4x per the incident description). You have access to the actual failed run's log (forge run `37628464539`, job "Deploy forge-baas (isolated)") if you want the literal message shape to craft a precise fixture — otherwise build a representative synthetic fixture.
- Reference existing sibling pattern: `pkg/api/logger/redact.go` + its `RedactingWriter`/`Redact(s string) string` helper is the established house style for this kind of post-hoc string redaction in this repo (regex-based, applied just before the line is printed). Consider whether S3's fix is best expressed as (a) a new standalone detector in `pkg/clouds/pulumi/events.go`, or (b) an addition to `pkg/api/logger/redact.go` that `procDiagnosticEvent` calls into alongside/instead of relying on `logger.Info`'s existing blanket `Redact()` call — your call, but keep the chokepoint single and generic as the brief demands.
- Vendored SDK location for the signature constants: `github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig` — `sig.Key = "4dabf18193072939515e22adb298388d"`, `sig.Secret = "1b47061264138c4ac30d75fd1eb44270"`. Import these rather than re-hardcoding the magic strings, for maintainability and to make the test intent obvious.
- `pkg/clouds/pulumi/aws/provider_test.go` is a good model for this repo's Gomega-based test style (`RegisterTestingT(t)`, table-driven where useful).
- Do not touch `pkg/clouds/pulumi/aws/provider.go` in this dispatch (see CUT #9) — mention it as a follow-up if you think it's still worth doing, but it's not required for S3's DoD.

## 6. Notable flag for Developer (operational, not a design concern)

This is the **3rd dispatch attempt** of this exact slice. Two prior Developer-character turns crashlooped (`exit status 143`, "claude emitted no output beyond init for 5m0s") before writing any code, with clean Architect handoffs preceding both. If you are the Developer and you hit a similar hang at turn startup, **report it plainly as the known pattern** (cite forge run ids of the prior 2 attempts if visible to you via `list_runs`) rather than guessing at a workaround or silently retrying indefinitely — this is likely an infra issue specific to this slice/repo combination, worth an explicit escalation note in your own handoff so the Roadmap Driver can decide whether a 4th attempt is worthwhile.

## 7. Tests run

N/A — PM turn, no code changes to test.

## 8. Verdict

`slice_id: aws-provider-key-in-deploy-log` (roadmap item reference carried through per Main Forge SDLC convention, for downstream next-slice resolution).

**Verdict:** signoff