[thinking] # PM Handoff — `aws-provider-key-in-deploy-log` (S3: redact diagnostic-event forwarding)

`slice_id: aws-provider-key-in-deploy-log`

## 0. Context recap (re-dispatch)

This is a re-dispatch after run `b0de022e` died to an **executor crashloop** during the Developer turn (infra flake — no code defect). The prior Architect handoff signed off cleanly before the crash; per the brief I'm re-deriving the plan from scratch rather than assuming that artifact still exists.

**Repo-level finding that changes the shape of this fix** (verified by reading the actual files, not assumed):

- `pkg/clouds/pulumi/events.go:75` — `procDiagnosticEvent` calls `p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", ..., evt.DiagnosticEvent.Message)`.
- `pkg/api/logger/logger.go` — **every** `Logger.Info/Warn/Error/Debug` call already routes through `Redact(...)` before `fmt.Println` (see `(l logger) println`: `fmt.Println(Redact(fmt.Sprintf(...) + fmt.Sprintf(format, a...)))`). This is already the one universal log chokepoint in the codebase, and `procDiagnosticEvent` already goes through it.
- `pkg/api/logger/redact.go` (+`redact_test.go`) is a mature, tested redactor — but it is scoped **only** to private-key-shaped material (PEM/PGP blocks, `private_key_id`, base64 runs ≥100 chars that decode to `PRIVATE KEY`). It has no concept of Pulumi's secret envelope and would not touch an AWS access-key/secret pair (too short to hit the 100-char base64 floor, not a PEM block).
- Pulumi's secret envelope signatures are public, verified from `pulumi/pulumi` upstream source (`sdk/go/common/resource/sig/sig.go`, github.com/pulumi/pulumi, fetched this turn): `Key = "4dabf18193072939515e22adb298388d"`, `Secret = "1b47061264138c4ac30d75fd1eb44270"`. On the wire a secret value serializes as an object carrying both signatures plus a `"value"` field holding the plaintext.
- `pkg/clouds/pulumi/aws/provider.go:96-99` (`applyAWSProviderCreds`) is explicitly OUT of scope per the brief (S1 already fixed it via dedicated `sc-deploy` IAM user; S2 — key revocation — is an operator call).

**Implication for Architect:** the brief names `events.go:75` as the fix point, but the actual generic, provider-agnostic chokepoint for "any secret property of any resource/provider" is `Redact()` in `pkg/api/logger/redact.go` — it's already on every log line, already has an established test pattern, and doesn't require every call site (incl. future providers) to remember to invoke a new helper. Recommended approach: **extend `Redact()` with a Pulumi-secret-envelope case**, which transitively fixes `procDiagnosticEvent` (and every other logger call) without touching `events.go` at all — satisfying "chokepoint, not per-provider" more completely than a local patch in `events.go` would. If Architect has a reason to prefer a local wrapper in `events.go` instead (e.g. to keep `Redact()`'s blast radius narrow), that's a legitimate call — but it must not regress to "each provider marks its own inputs," and must still pass through the same AC set below.

## 1. Requirements (restated from brief)

1. Deploy logs from a current SC binary must contain **no credential material for any provider**, even when a diagnostic event's raw gRPC payload carries Pulumi's secret envelope (`4dabf181…` / `1b470612…` signatures) around a plaintext value.
2. The fix must be a property of the **chokepoint** (diagnostic-event forwarding / the shared logger redaction path) — not something each provider has to opt into.
3. Add/extend unit tests covering: (a) a diagnostic message containing the secret-envelope signature is redacted; (b) a message with no secret envelope passes through byte-for-byte unchanged; (c) a realistic `getResource Invoke RPC finished`-shaped message (the leaked message's family) is redacted.
4. Full test suite for the touched package(s) passes; `go build ./...` and `go vet ./...` succeed.
5. This dispatch is also the first live proof of the `api` repo's newly-fixed auto-merge path (fixed 2026-10-15, commit `00c06a29`, tracked under `api-repo-branch-protection-blocks-sdlc-automerge`) — expect a normal PR to be evaluated for auto-merge without an escalation purely on branch-protection grounds. This is an observational note for DevOps/the engine, not an engineering task for this chain.

## 2. Acceptance criteria

| AC# | Criterion | Notes for downstream |
|---|---|---|
| AC1 | A diagnostic message containing Pulumi's secret envelope (`4dabf18193072939515e22adb298388d` / `1b47061264138c4ac30d75fd1eb44270` adjacency around a `"value"` field) is redacted before it reaches stdout/the Actions log, regardless of which provider/resource emitted it. | Unit test with a synthetic envelope modeled on the real sig constants (verified above). No real credential value should ever appear in the test fixture — synthesize a fake-looking AWS-shaped key for the test, not a real one. |
| AC2 | A message with no secret envelope passes through **byte-for-byte unchanged** (no accidental new false-positive redaction of ordinary diagnostic text, URNs, resource type tokens, etc.). | Regression test alongside existing `TestRedactLeavesOtherTextAlone` / `TestRedactLeavesOtherBase64Alone` pattern in `redact_test.go`. |
| AC3 | A realistic sample of the `getResource Invoke RPC finished` message family (the one that actually leaked) is redacted. Exact historical payload isn't in the repo — construct a representative synthetic sample from the known Pulumi RPC/secret-envelope shape (tok + args/response JSON wrapping a secret envelope), clearly marked as synthetic/representative in the test, not claimed as the verbatim incident payload. | New test in `pkg/clouds/pulumi` (events_test.go, doesn't currently exist) and/or `pkg/api/logger` (redact_test.go), per Architect's chosen fix location. |
| AC4 | Fix is the chokepoint's property: works for AWS, GCP, Yandex, and any future provider without each one marking inputs. | Verify no per-provider changes are introduced as part of this slice (provider.go untouched — S1/S2 stay out of scope). |
| AC5 | Full test suite for touched package(s) passes. | `go test ./pkg/api/logger/... ./pkg/clouds/pulumi/...` (scope to touched packages per the turn-size guidance — do not run full `go test ./...` on this large repo). |
| AC6 | `go build ./...` and `go vet ./...` succeed. | Run at repo root. |
| AC7 | No real/live credential value appears anywhere in code, tests, commit messages, or handoffs — only synthetic/fake-shaped fixtures. | Secret-hygiene self-check before every handoff in this chain. |

## 3. Scope

| Item | Label | Rationale |
|---|---|---|
| Redact Pulumi secret-envelope-wrapped values before any diagnostic/engine-event message reaches the logger output | MUST | Core of the brief; the actual leak vector. |
| Make the fix chokepoint-level (logger `Redact()` or the shared `procDiagnosticEvent`/event-forwarding path) rather than per-provider | MUST | Explicit brief requirement; also the only way to protect GCP/Yandex/future providers without a follow-up slice per provider. |
| Unit tests: envelope-present / envelope-absent / realistic RPC-shaped sample | MUST | Explicit brief requirement (AC3). |
| `go build ./...` + `go vet ./...` + scoped `go test` green | MUST | Explicit brief requirement (AC4/AC5 of brief, AC5/AC6 above). |
| Extending redaction to also cover other event types (`ResOutputsEvent`, `ResourcePreEvent`, policy events) that already call `p.logger.Info` and already get the same `Redact()` pass | NICE-TO-HAVE | They already benefit for free if the fix lands in `Redact()` (chokepoint); if Architect instead does a local `events.go` wrapper, call out explicitly whether these other event handlers are covered — don't leave a silent gap. |
| Revoking the already-leaked key (S2) | CUT | Explicit brief exclusion — operator judgement call, out of scope for this dispatch. |
| Switching AWS provider creds to marked/secret `sdk.Output` types in `provider.go` (S1-adjacent hardening) | CUT | Explicitly called out in the brief as insufficient alone and not part of this dispatch's scope; S1 (dedicated `sc-deploy` IAM user) is already done. |
| Broader secret-scanning/DLP across all SC logging surfaces beyond diagnostic-event forwarding | CUT | Out of scope — ruthlessly minimal bet: fix the one chokepoint named in the brief, not a general log-scrubbing audit. |

## 4. Handoff guidance for Architect

- Start from `pkg/api/logger/redact.go` and its test file `redact_test.go` — it is the established, tested pattern for "detect a secret shape → redact it → verify surrounding text untouched." Follow its existing structure (regex-based detection + a `TestRedactLeavesOtherTextAlone`-style negative test) rather than inventing a new mechanism.
- Decide and document explicitly: extend `Redact()` itself (recommended — it's already the universal chokepoint on every `logger.Info/Warn/Error/Debug` call, so it fixes `procDiagnosticEvent` and everything else transparently) **vs.** a local redaction wrapper placed in `events.go`'s `procDiagnosticEvent` per the brief's literal file:line pointer. Either is acceptable as long as AC1–AC4 hold; state the choice and why in your handoff so Developer doesn't have to re-derive it.
- The secret envelope's `"value"` field can itself be a nested object/array in principle (a secret resource reference) — flat-string-value is the common/realistic case per the incident report (AWS secret key as a string) and is what the tests must cover; treat deeper nesting as a documented non-goal unless trivial to also catch.
- No live AWS key, token, or other real credential value should appear anywhere in your handoff or in committed test fixtures — construct synthetic fixtures (e.g. `AKIAFAKEFAKEFAKEFAKE` / a clearly-fake 40-char string) the same way `redact_test.go` already uses synthetic generated keys.
- `events_test.go` doesn't exist yet in `pkg/clouds/pulumi` — Developer will be creating it, not extending it.

## 5. Files touched

No code mutations in this turn — PM planning turn only.

| Path | Lines added / removed | What changed |
|---|---|---|
| (none) | — | — |

## 6. Tests run

Not applicable — no code mutated this turn. Verification performed: read `pkg/clouds/pulumi/events.go`, `pkg/clouds/pulumi/aws/provider.go`, `pkg/api/logger/redact.go`, `pkg/api/logger/logger.go`, `pkg/api/logger/redact_test.go` directly from the `api` repo (`main` branch) via `read_repository_file`/`search_repository_code`; verified Pulumi's secret-envelope signature constants against the public upstream source `pulumi/pulumi/sdk/go/common/resource/sig/sig.go` via `fetch_url`.

## 7. Verdict

**Verdict:** signoff