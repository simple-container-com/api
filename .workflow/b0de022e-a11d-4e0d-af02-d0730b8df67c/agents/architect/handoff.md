[thinking] # Architect Handoff — `aws-provider-key-in-deploy-log`: redact secrets in diagnostic logging

## 1. Commit + branch identity

No code mutations in this turn. Architect role does not write code — this handoff is a design decision + implementation spec for the Developer.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none — design only)* | — | — |

## 3. Architectural finding — correcting the brief's root-cause framing

I read the actual code this turn (not inferred):

- `pkg/clouds/pulumi/events.go` (sha `00aedc4d4260…`) — `procDiagnosticEvent`:
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
- `pkg/api/logger/logger.go` (sha `5996c09c358e…`) — `Info` → `println`:
  ```go
  func (l logger) println(ctx context.Context, levelString, format string, a ...any) {
      ...
      fmt.Println(Redact(fmt.Sprintf("%s%s: ", datePrefix, levelString) + fmt.Sprintf(format, a...)))
  }
  ```
- `pkg/api/logger/redact.go` (sha `551c528048…`) — a **pre-existing, already-wired, already-tested** `Redact()` helper (landed PR #429, commit `dc6a461f5e58`, 2026-10-09) that every `logger.Info/Warn/Error/Debug` call passes through before printing.

**The brief's claim that `procDiagnosticEvent` "forwards the message verbatim, bypassing Pulumi's own redaction" is not quite right — it bypasses nothing of its own; every `Info()` call already runs through `Redact()`.** The real gap is narrower and more useful to know: **`Redact()`'s current pattern set only recognizes PEM/private-key-shaped material** (`-----BEGIN ... PRIVATE KEY-----` blocks, base64 blobs that decode to `PRIVATE KEY`, `private_key_id` fields). It has zero patterns for:
- AWS access-key / secret-key shapes,
- Pulumi's secret-envelope signature pair (`sig.Key` = `4dabf18193072939515e22adb298388d`, `sig.Secret` = `1b47061264138c4ac30d75fd1eb44270` — both PUBLIC Pulumi SDK constants, confirmed against `pulumi/sdk/go/common/resource/sig/sig.go` upstream, safe to reference by value).

That's why the AWS key sailed through 4× in one log: it went through `Redact()`, and `Redact()` let it pass because it didn't recognize the shape.

## 4. Decision record

**Decision: extend the existing `Redact()` in `pkg/api/logger/redact.go` with new pattern families, rather than writing new, parallel redaction logic local to `procDiagnosticEvent`.**

| Option | Pros | Cons |
|---|---|---|
| **A. Extend shared `Redact()`** (chosen) | Single chokepoint already wired into *every* logger call across the whole binary (CLI errors, previews, diagnostics, policy, failures) — strictly stronger than the brief's "generic across every provider/cloud" requirement, since it also covers event handlers outside `procDiagnosticEvent` that the brief scoped out, for free, with zero extra wiring. Already has a mature, passing test suite (`redact_test.go`) to extend incrementally. DRY — no second redaction implementation to keep in sync. | Slightly broader blast radius than a `procDiagnosticEvent`-local helper (mitigated: additive regexes only, existing tests must stay green). |
| **B. New `redactSecretsFromDiagnostic()` local to `events.go`** (PM's suggested shape) | Narrowly scoped, easy to point at in review. | Duplicates concerns `Redact()` already owns; a future second leak path (e.g. `procResourceFailedEvent`, which brief explicitly excludes from scope but which 1-year-from-now someone will want covered) would need the *same* fix applied *again*. Violates the DoD's own framing ("not of each provider/call-site remembering to mark its inputs") — a second bespoke redactor is exactly the kind of per-call-site remembering the brief is trying to kill. |

Going with **A**. This keeps `procDiagnosticEvent` itself at **zero diff** — the fix lands entirely in `pkg/api/logger/redact.go` + its test file, which is the smallest possible blameable surface and the cleanest diff for the branch-protection open-PR path this run is testing.

```
procDiagnosticEvent(evt)
        │
        ▼
  p.logger.Info(ctx, fmt, args...)      ← every Logger method funnels here
        │
        ▼
  logger.println(...)
        │
        ▼
  Redact(formatted string)   ← ONE chokepoint, extend here
        │
        ▼
  fmt.Println(...)           ← what actually reaches the Actions log
```

## 5. Implementation spec for Developer

Add to `pkg/api/logger/redact.go`, alongside the existing `privateKeyBlock`/`privateKeyTail`/`privateKeyID`/`base64Run` vars, and apply inside `Redact()`:

**5.1 Pulumi secret envelope.** Pulumi serializes a secret as (roughly, possibly JSON-escaped 1-3× the same way `private_key_id` already is handled in this file):
```json
{"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":"<secret>"}
```
Add a regex anchored on the two signature constants (public, non-sensitive) that captures and redacts the `value` field's immediate scalar, tolerating the same escape/quote variations `privateKeyID` already tolerates (`keySep`-style escaping). Known limitation to document in a code comment (matches this file's existing style of documenting limitations, e.g. "Base64 wrapped over several lines is not caught"): a deeply nested non-scalar `value` (object/array) won't be fully redacted by a single-pass regex — flag as a follow-up if it surfaces in practice, don't block this fix on it.

**5.2 AWS Access Key ID.** Prefix list is AWS-documented and public (not a secret in itself — a fixed-prefix identifier):
```go
awsAccessKeyID = regexp.MustCompile(`\b(?:AKIA|ASIA|AIDA|AROA|AGPA|ANPA|ANVA|ASCA)[A-Z0-9]{16}\b`)
```
Redact the whole match.

**5.3 AWS Secret Access Key — context-anchored, not bare-pattern.** A bare 40-char base64-ish string is too ambiguous to blanket-redact (would collide with image digests, hashes, etc. — see `TestRedactLeavesOtherBase64Alone` which already locks in that bare base64 must survive). Anchor on the key name:
```go
awsSecretKeyKV = regexp.MustCompile(`(?i)((?:aws_)?secret(?:_access)?key"?\s*[:=]\s*"?)[A-Za-z0-9/+=]{40}`)
```
Redact only the value, keep group 1 (the label) — same pattern `privateKeyID` already uses (`"${1}"+redacted`).

**5.4 Generic credential key=value backstop** (brief explicitly asks for this, to catch `applyAWSProviderCreds`'s unmarked `args.AccessKey`/`args.SecretKey` showing up in a diff/diagnostic even without the Pulumi secret envelope):
```go
genericSecretKV = regexp.MustCompile(`(?i)((?:secret|password|token|api[_-]?key)"?\s*[:=]\s*"?)\S{8,}`)
```
Flag to Developer: this is deliberately biased toward over-redaction (false positives are the acceptable failure mode for a security backstop; false negatives are not). Run it against the full existing `redact_test.go` suite including `TestRedactLeavesOtherTextAlone` / `TestRedactLeavesOtherBase64Alone` to make sure nothing currently-allowed starts getting clobbered — if `client_email: deployer@...gserviceaccount.com` or similar starts matching, tighten the key-name alternation rather than weakening the length threshold.

**5.5 `applyAWSProviderCreds` (pkg/clouds/pulumi/aws/provider.go:96-99).** Optional, zero-risk drive-by only if trivial: wrap `args.AccessKey`/`args.SecretKey` in `sdk.ToSecret(...)`. Not required for DoD (brief: "the real fix belongs in procDiagnosticEvent" — here, in the shared `Redact()`). Skip if it risks a provider-diff regression on existing stacks; note it as a follow-up instead.

## 6. Test plan for Developer (maps to AC2)

1. **`pkg/api/logger/redact_test.go`** — add cases to the existing table-driven style:
   - A Pulumi secret-envelope JSON string (fabricated placeholder value, e.g. `AKIAFAKEFAKEFAKEFAKE` / `fake40charsecretkeyvalueFAKEFAKEFAKEFAKE`) → `Redact()` output contains no fragment of the placeholder and does contain `redacted`.
   - A raw `AKIA...`/secret-key-shaped key=value pair (fabricated) → same assertion.
   - Re-run full existing suite (`TestRedactLeavesOtherTextAlone`, `TestRedactLeavesOtherBase64Alone`, etc.) to confirm no regression.
2. **New `pkg/clouds/pulumi/events_test.go`** — closes the loop on the brief's literal AC2 wording ("mocked diagnostic message... procDiagnosticEvent"): construct `events.EngineEvent{DiagnosticEvent: &apitype.DiagnosticEvent{URN: ..., Message: <string containing the fabricated secret-envelope + AWS-shaped fixture>}}`, call `(&pulumi{logger: logger.New()}).procDiagnosticEvent(ctx, evt)` with stdout captured (same pattern as `TestLoggerRedactsWhatItPrints` in `redact_test.go` — reuse or re-implement `captureStdout`), assert the captured output contains no fixture fragment. This is pure proof-of-wiring (the redaction logic itself is unit-tested in #1) and should need zero production-code changes to `events.go`.
3. **AC3**: `go build ./pkg/api/logger/... ./pkg/clouds/pulumi/... && go test ./pkg/api/logger/... ./pkg/clouds/pulumi/...` — Developer to run and paste tail of output in their handoff.
4. **AC4** (live proof): operator/QA-owed, per PM's handoff — do not fabricate. If a live isolated `forge-baas` deploy is triggered and its Actions log is grepped clean, QA should be the one asserting that with a pasted grep result; otherwise flag FAIL-blocked-operator.

## 7. Scope guardrails carried forward (do not expand)

- No IAM changes, no credential revocation (S1/S2 — done / operator call, per brief).
- No redaction added to `procResourcePreEvent`/`procResourceOutputsEvent`/`procResourceFailedEvent`/`procPolicyEvent` as *new call sites* — but note they already benefit for free via `Redact()` since they all call `p.logger.Info`/`Warn`. No extra diff needed to get that coverage; it's an automatic consequence of Decision §4, not scope creep.
- No separate docs/design file for this change — the fix is two small, additive regex sets in an already-documented, already-tested file; a standalone design doc would inflate the diff against the brief's own stated concern about keeping this run's diff clean for the branch-protection open-PR fallback test. This handoff is the design record.
- Repo-specific process note (carried from brief, unchanged): `simple-container-com/api` requires 2 human reviews — expect an **open PR**, not an auto-merge, as the correct successful outcome for this run.

## 8. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `procDiagnosticEvent`/its logging path redacts secret-shaped values before logging at Info level | FAIL — not yet implemented | Design-complete, spec in §5 above; implementation is Developer's turn. Confirmed the mechanism exists and the chokepoint (`Redact()` inside `logger.println`, called from every `Info()`) is real, by reading `events.go`, `logger.go`, `redact.go` this turn (SHAs cited in §3) |
| AC2 | Unit test covers redaction against a mocked diagnostic message containing a Pulumi secret-envelope-wrapped value | FAIL — not yet implemented | Test plan specified in §6 (both `redact_test.go` additions and new `events_test.go`); not yet written — Developer's turn |
| AC3 | `go build ./... && go test ./...` scoped to touched package green | FAIL — not yet run | No code written yet this turn; exact commands specified in §6.3 for Developer to run and report |
| AC4 | Live or named-operator-owed confirmation that a real deploy log no longer contains credential material | FAIL-blocked-operator (expected at this phase) | Requires a live/re-run isolated `forge-baas` deploy against a current SC binary — not available to Architect; QA/DevOps to execute post-merge per PM's handoff §5 |

## 9. Tests run

None — Architect turn, no code touched. Commands for Developer to run are specified in §6.3 above.

## 10. Verdict

**Verdict:** signoff