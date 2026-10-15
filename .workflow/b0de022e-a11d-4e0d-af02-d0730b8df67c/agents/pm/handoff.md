[thinking] # PM Handoff — `aws-provider-key-in-deploy-log`: redact secrets in `procDiagnosticEvent`

## 1. Commit + branch identity

No code mutations in this turn. PM role does not write code.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none — planning only)* | — | — |

## 3. Restated requirement

A failed isolated `forge-baas` deploy printed a live AWS access key + secret in plaintext, 4× in one Actions log, inside a `getResource Invoke RPC finished` gRPC diagnostic. Marking a Pulumi resource property `secret` does **not** stop this because the leak path bypasses Pulumi's own redaction: `pkg/clouds/pulumi/events.go:75`, `procDiagnosticEvent`, forwards `evt.DiagnosticEvent.Message` — the raw diagnostic string straight off the gRPC wire — to `p.logger.Info(...)` verbatim. Pulumi's secret envelope (`PropertyValue` wrapped as `map[string]any{"4dabf181...": "1b470612...", "value": <secret>}`, per `sig.Key`/`sig.Secret` in `pulumi/sdk/.../resource/sig/sig.go`) never gets a chance to apply its own summarizer redaction at this chokepoint, because the message is pre-serialized plain text, not a structured `PropertyValue`.

Confirmed in code (read this turn):
- `pkg/clouds/pulumi/events.go` lines ~74-76:
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
  This is the single chokepoint every provider's diagnostics pass through — confirmed only one call site in the repo (`search_repository_code` → 1 hit).
- `pkg/clouds/pulumi/aws/provider.go` lines 96-99, `applyAWSProviderCreds`: passes `args.AccessKey`/`args.SecretKey` as plain unmarked `sdk.String(...)`. This is AWS-specific and secondary — noted per brief, NOT the fix target.
- Pulumi secret-envelope signatures (public, from `pulumi/sdk/go/common/resource/sig/sig.go`): `sig.Key = "4dabf18193072939515e22adb298388d"`, `sig.Secret = "1b47061264138c4ac30d75fd1eb44270"`. These are PUBLIC, well-known Pulumi constants — safe to reference by value in code/tests (not credentials).

## 4. Scope (MoSCoW)

| Item | Priority | Notes |
|---|---|---|
| Redact secret-shaped values in `procDiagnosticEvent` before `p.logger.Info` | **MUST** | The one chokepoint; brief's core ask |
| Detect Pulumi secret-envelope JSON fragments (`4dabf181.../1b470612...`) inside the raw message string | **MUST** | These signatures can appear verbatim in a serialized diagnostic even though the message is a flat string, not a structured map — regex/substring scan is the pragmatic approach at this layer |
| Detect common raw-credential shapes (AWS access key `AKIA[0-9A-Z]{16}`, generic `secret`/`key`/`token` key=value patterns) as a defense-in-depth backstop | **MUST** | Brief explicitly calls for "common credential patterns" — covers the case where a provider never marks the field secret at all (e.g. `applyAWSProviderCreds`'s unmarked args) |
| Unit test: mocked diagnostic message containing a secret-envelope-wrapped value → asserts the logged output is redacted | **MUST** | AC2 |
| `go build ./... && go test ./...` scoped to `pkg/clouds/pulumi/...` green | **MUST** | AC3 |
| Fix `applyAWSProviderCreds` to mark `AccessKey`/`SecretKey` as Pulumi secret inputs | **NICE-TO-HAVE** | Brief explicitly says "note it, the real fix belongs in `procDiagnosticEvent`" — do not let this expand scope; a drive-by one-liner (`sdk.ToSecret(...)`) is acceptable if trivial, but not required for DoD |
| Redacting `ResourcePreEvent`/`ResOutputsEvent`/`ResOpFailedEvent`/`PolicyEvent` diagnostics too | **CUT** | Out of scope per brief: "S3 ONLY" is `procDiagnosticEvent`; other event handlers consume structured `apitype.PropertyDiff`/outputs which Pulumi already redacts upstream via its own secret marking — not the reported leak path |
| Touching IAM / revoking the leaked key (S1/S2) | **CUT — explicitly forbidden by brief** | "do NOT touch IAM, do NOT revoke any credential" |
| Live redeploy proof against `forge-baas` isolated stack | **MUST (but operator/QA-owed, not dev-owed)** | AC4 requires live-or-named-operator confirmation — see below |

## 5. Acceptance criteria (restated + plan for verification)

| AC# | Criterion | How Architect/Dev should satisfy it |
|---|---|---|
| AC1 | `procDiagnosticEvent` redacts secret-shaped values before logging at Info level | Add a `redactSecrets(msg string) string` (or similarly named) helper invoked inside `procDiagnosticEvent` before the `p.logger.Info` call. Must catch (a) Pulumi secret-envelope JSON fragments containing `sig.Key`/`sig.Secret` constants, and (b) common raw credential patterns (AWS `AKIA...`/`ASIA...` access-key prefixes + adjacent 40-char secret-looking strings, generic high-entropy `key`/`secret`/`token`/`password` key=value pairs). Redact to a fixed placeholder like `***REDACTED***`, never partially (no "first 4 chars shown") |
| AC2 | Unit test covers redaction against a mocked diagnostic message containing a Pulumi secret-envelope-wrapped value | New test in `pkg/clouds/pulumi/events_test.go` (new file) constructing an `events.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{Message: <string containing a JSON-serialized secret envelope with sig.Key/sig.Secret and a fake-but-realistic AWS-shaped value>}}`, calling `procDiagnosticEvent` (or the extracted redaction func directly — prefer testing the pure function for determinism) and asserting the captured log output contains no credential substring and does contain the redaction placeholder. Use a fabricated placeholder key value in the test fixture (e.g. `AKIAFAKEFAKEFAKEFAKE` / `fake-secret-...`), never a real-looking unlabelled high-entropy string that could later be mistaken for a leaked real credential |
| AC3 | `go build ./... && go test ./...` scoped to touched package green | Run `go build ./pkg/clouds/pulumi/... && go test ./pkg/clouds/pulumi/...` (scope to the touched package per repo's own guidance on not running full-repo builds) and paste the tail of output in the Dev/QA handoff |
| AC4 | Live or named-operator-owed confirmation that a real deploy log no longer contains credential material | This is almost certainly **operator/QA-owed**: it requires triggering a real (or re-run) isolated `forge-baas` deploy against a current SC binary and grepping the resulting Actions log. Dev/Architect should NOT attempt to fabricate this — QA should, if QA holds CI/Actions access; otherwise flag as operator-owed in the DevOps/QA handoff rather than hand-waving a PASS |

## 6. Out-of-scope / explicit non-goals

- No IAM changes, no credential revocation (S1/S2 — already done/operator call per brief).
- No redaction logic added to other event handlers (`procResourcePreEvent`, `procResourceOutputsEvent`, `procResourceFailedEvent`, `procPolicyEvent`) — brief scopes this to the diagnostic chokepoint only; expanding will inflate the diff and risk the "clean, safe diff → open PR" path the brief explicitly calls out for this repo's branch-protection auto-merge fallback.
- No change to how Pulumi itself marks resource properties secret across every provider — this fix is a logging-layer backstop, not a provider-input-marking campaign.

## 7. Repo-specific process note (carried forward from brief)

`simple-container-com/api` requires 2 human reviews before merge (branch-protection ruleset) — SDLC auto-merge is blocked by design. Per the brief, forge-conductor's merge-evidence logic (commit `00c06a29`, deployed today) should now detect this and fall back to an **open PR**, not escalate on "auto-merge did not land." Architect/Dev/QA/DevOps: expect and accept an open-PR outcome as success, provided the diff is clean and ACs are met — do not treat "PR not merged" as a failure signal for this run.

## 8. Handoff guidance for Architect

1. Confirm the extraction point: prefer a small, independently-unit-testable pure function (`redactSecretsFromDiagnostic(msg string) string` or similar) over inlining regex logic directly in `procDiagnosticEvent` — this keeps AC2's unit test clean and deterministic, and matches the brief's "property of `procDiagnosticEvent` itself" DoD note (the function is still *called from* `procDiagnosticEvent`, satisfying that generic-chokepoint requirement, while staying testable).
2. Pick/confirm pattern set with Dev before implementation: (a) Pulumi secret envelope — match `sig.Key` (`4dabf18193072939515e22adb298388d`) and `sig.Secret` (`1b47061264138c4ac30d75fd1eb44270`) substrings in the raw message and redact the enclosing value; (b) generic credential-shaped regexes (AWS access key prefix `AKIA|ASIA`, `aws_secret_access_key`-style key=value, generic `(?i)(secret|password|token|apikey)\s*[:=]\s*\S+`). Keep the pattern list small and documented — this is a backstop, not a DLP engine.
3. Do not scope-creep into `applyAWSProviderCreds` beyond a trivial drive-by fix (wrapping the two args in `sdk.ToSecret`) if Dev judges it zero-risk; otherwise leave it as a follow-up note in the handoff, per brief's own framing ("note it... the real fix belongs in procDiagnosticEvent").
4. AC4 (live proof) should be flagged FAIL-blocked-operator in Dev/QA's handoff if no live deploy is triggered in this run — do not claim PASS by inference from a code read.

## 9. Tests run

None — PM turn, no code touched. Build/test commands specified above are for Architect/Dev to run and report.

## 10. Verdict

**Verdict:** signoff