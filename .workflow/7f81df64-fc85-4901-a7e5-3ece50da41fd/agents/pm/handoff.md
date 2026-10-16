[thinking] # PM Handoff — S3: Redact Pulumi secret-envelope values in `procDiagnosticEvent`

## 0. Context digest (read directly from `simple-container-com/api`, default branch, current HEAD)

- Target chokepoint confirmed live: `pkg/clouds/pulumi/events.go`, function `procDiagnosticEvent` (currently ~line 56 in the file I read, not 75 as the brief's line number suggests — the file has moved since the brief was written; Architect/Developer should locate by function name, not line number):
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
  `evt.DiagnosticEvent.Message` is a plain `string` (`apitype.DiagnosticEvent.Message`), forwarded verbatim into `p.logger.Info`.

- **Existing redaction is narrower than this problem.** `pkg/api/logger/redact.go`'s `Redact()` (used by every `logger.Info`/`Error` call, confirmed in `pkg/api/logger/logger.go`) targets **private-key material specifically**: PEM blocks, PEM tails, `private_key_id` hex ids, and base64 runs that decode to a string containing `"PRIVATE KEY"`. It has **no concept of Pulumi's generic secret envelope** — a plain string value (e.g. an AWS access key or secret key) wrapped as `{"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":"<secret>"}` (sig constants confirmed from `pulumi/sdk/v3/go/common/resource/sig` at the pinned `go.mod` version v3.245.0: `sig.Key = "4dabf18193072939515e22adb298388d"`, `sig.Secret = "1b47061264138c4ac30d75fd1eb44270"`) will pass through `Redact()` untouched today, because it isn't PEM-shaped and doesn't decode to text containing "PRIVATE KEY".
  - This is exactly consistent with the brief: an AWS access key + secret key are plain strings, not PEM blocks, so the current logger-level redaction doesn't catch them — the gap is real and specifically about **secret-marked (not just key-shaped) values**.

- Existing test precedent to follow: `pkg/clouds/pulumi/preview_redact_test.go` (`TestPreviewAndUpdateSummariesAreRedacted`) already tests that `p.toPreviewResult`/`toUpdateResult` summaries are redacted — same package, same `pulumi` receiver type, same test style (construct a real key/secret, assert fragments don't survive). `pkg/api/logger/redact_test.go` is the analogous unit-test style for the `Redact()` function itself. Both are strong templates for the new test(s).

- `go.mod` pins `github.com/pulumi/pulumi/sdk/v3 v3.245.0` — Architect/Developer should design against that exact version's `events`/`apitype`/`resource`/`sig` packages (verified above), not assume behavior from a different Pulumi SDK version.

## 1. Requirements (restated from brief)

- **MUST** — Redact any secret-marked value that a Pulumi diagnostic event's `Message` carries, before it reaches `p.logger.Info` in `procDiagnosticEvent` (`pkg/clouds/pulumi/events.go`).
- **MUST** — Fix at the `procDiagnosticEvent` chokepoint itself (or a helper it calls), not per-provider — so it covers AWS, GCP, Yandex, every current and future provider without each one remembering to mark inputs correctly.
- **MUST** — No regression to normal (non-secret) diagnostic messages — they must still print in full and unchanged.
- **MUST** — Unit test: a diagnostic message containing a Pulumi secret-envelope-wrapped value → redacted.
- **MUST** — Unit test: a diagnostic message with no secret material → passes through byte-for-byte unchanged.
- **CUT (explicitly out of scope per brief)**:
  - S1 (IAM user replacement) — already shipped.
  - S2 (revoking the leaked key) — deliberate operator call, not this slice.
  - `pkg/clouds/pulumi/aws/provider.go:96-99` (AWS-specific plain-string issue) — a different, narrower fix; not this slice.
  - Branch-protection / auto-merge behavior on `api` — separate, already-addressed roadmap item (`api-repo-branch-protection-blocks-sdlc-automerge`); do not attempt to fix, just report PR number + `mergeable_state` if a PR opens.

## 2. Acceptance criteria (numbered, for Architect/Developer/QA to verify against)

| AC# | Criterion |
|---|---|
| AC1 | A deploy log produced by a current SC binary contains no credential material for any provider's secret-marked properties, even when a diagnostic event carries them (generic, not AWS-only). |
| AC2 | The redaction lives in `procDiagnosticEvent` itself (or something it calls) — i.e. it is a property of the chokepoint, not of each provider correctly marking its own inputs. |
| AC3 | Unit test: a diagnostic message containing a Pulumi secret-envelope-wrapped value (construct with real `sig.Key`/`sig.Secret` markers per pinned SDK v3.245.0, or via the SDK's own secret-property serialization) gets redacted — no fragment of the secret value survives. |
| AC4 | Unit test: a diagnostic message with no secret material passes through **unchanged** (byte-for-byte) — regression guard against over-redaction. |
| AC5 | Non-secret, non-key text in diagnostic messages (e.g. ordinary resource names, URNs, "Resources: N to update") is not altered — reuse the existing `TestRedactLeavesOtherTextAlone`/`TestPreviewAndUpdateSummariesAreRedacted` style of assertion. |
| AC6 | `go build ./pkg/clouds/pulumi/...` and `go test ./pkg/clouds/pulumi/... ./pkg/api/logger/...` pass clean. |
| AC7 | If a PR actually opens against `api`, Developer/DevOps notes PR number + `mergeable_state` in their handoff (first live proof since branch-protection fix; informational only, not a blocker). |

## 3. Key open design question for Architect

The existing `logger.Redact()` is pattern-matching on PEM/base64-decodes-to-PRIVATE-KEY shapes — it has no notion of "this value came from a Pulumi property marked secret." The Architect must decide and document one of:

- **(a)** Extend `procDiagnosticEvent` (or a new helper in `pkg/clouds/pulumi`) to recognize the Pulumi secret-envelope shape directly in the message text (JSON object carrying `sig.Key`/`sig.Secret`) and mask the associated value field generically — independent of whether the value looks like a private key, an AWS key, or anything else.
- **(b)** If the message is reliably JSON (or contains embeddable JSON fragments) rather than free text, parse and walk it structurally rather than regex-matching, to avoid both under- and over-redaction.

Either way: the fix must NOT special-case AWS access-key/secret-key shapes (that's the explicitly-cut `provider.go` fix) — it must key off Pulumi's own secret marker, which is provider-agnostic by construction.

## 4. Handoff to Architect

- Confirmed real file: `pkg/clouds/pulumi/events.go` (function `procDiagnosticEvent`), confirmed real existing redaction utility: `pkg/api/logger/redact.go` (`Redact()`), confirmed real existing test precedent: `pkg/clouds/pulumi/preview_redact_test.go`, `pkg/api/logger/redact_test.go`.
- Confirmed real Pulumi SDK version (`go.mod`): `github.com/pulumi/pulumi/sdk/v3 v3.245.0`. Secret-sig constants confirmed at that tag: `sig.Key = "4dabf18193072939515e22adb298388d"`, `sig.Secret = "1b47061264138c4ac30d75fd1eb44270"` (package `github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig`).
- `apitype.DiagnosticEvent.Message` is a plain `string` field — whatever representation of the secret envelope reaches it, it arrives as text, not a structured type. Architect needs to pin down (by reading the Pulumi automation-API event-emission path, or by reconstructing the shape from the forge-run-37628464539 incident description in the brief) exactly what that text looks like before choosing a matching strategy.
- Reuse, don't reinvent: if the new redaction logic is general enough, consider whether it belongs in `pkg/api/logger.Redact()` itself (so every log call benefits) vs. a `pulumi`-package-local helper called only from `procDiagnosticEvent` (brief's DoD explicitly only requires the latter — don't expand scope to touch the shared logger unless it's the cleaner fix with no side effects on unrelated call sites).
- Expect `mergeable_state: blocked` on the resulting PR against `api` (branch protection requires 2 human reviews) — normal, not a signal of failure. Do not attempt to fix that.
- Developer executor stability note (not a scope item, just an operational flag): 3 prior dispatches on this exact slice died with `exit status 143` crashloops in the Developer character's executor turn, after a clean Architect signoff each time. If you're the Developer on this run, treat any unusual turn-startup signal (hung MCP handshake, slow clone) as the known pattern, not a one-off — it has nothing to do with the design clarity, which has been clean 3/3 times.

## 5. Files touched

No code mutations in this turn.

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 6. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1–AC7 | (see table above) | N/A — PM turn produces plan only | These are the criteria Developer/QA must satisfy; not verifiable at PM stage. Design feasibility pre-checked: confirmed `procDiagnosticEvent`, `logger.Redact`, Pulumi SDK sig constants, and existing test patterns are all real, present, and consistent with the brief's description (see §0). |

## 7. Tests run

No code mutations in this turn — nothing to test. Test *requirements* for Developer are specified in AC3–AC6 above, templated on the real, existing `pkg/api/logger/redact_test.go` and `pkg/clouds/pulumi/preview_redact_test.go`.

**Verdict:** signoff