[thinking] # Architect Handoff — S3: Redact Pulumi secret-envelope values in `procDiagnosticEvent`

## 0. Scope confirmation

Design-only slice. Repo **is** accessible — I read real files at real SHAs (not synthesized). No PROVISIONAL marker needed; what follows is a concrete design ready for Developer to implement, not a hypothesis about files I couldn't see.

Confirmed by direct read (`read_repository_file`, `simple-container-com/api`, default branch):
- `pkg/clouds/pulumi/events.go` — sha `00aedc4d42601d82cb031435560d40b41e8843d9`, 3450 bytes. Current `procDiagnosticEvent`:
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
- `pkg/api/logger/redact.go` — sha `551c5280489c64a23c0e33a4c85aba374ea9cc83`. Confirmed: `Redact(s string) string` is regex/base64-decode based, targets PEM/private-key shapes only (`privateKeyBlock`, `privateKeyTail`, `privateKeyID`, `base64Run`+`holdsPrivateKey` sniff). It has **no** notion of Pulumi's JSON secret envelope.
- `pkg/api/logger/logger.go` — sha `5996c09c358ed540b882d589cd52cbe780de4d88`. Confirmed: `logger.println` already wraps every final line in `Redact(...)` before `fmt.Println` — this is the existing fleet-wide defense-in-depth layer, and it runs unconditionally regardless of what `procDiagnosticEvent` does.
- `pkg/clouds/pulumi/aws/provider.go` — sha `28e039e7eafa782f49b3632c1f77760ce073335a`. Confirmed untouched/out of scope per brief: `applyAWSProviderCreds` (lines ~96-99 in brief's citation) passes `sdk.String(accessKey)`/`sdk.String(secretAccessKey)` plain — this stays as-is, not part of S3.
- `go.mod` pins `github.com/pulumi/pulumi/sdk/v3 v3.245.0` exactly.
- Pulumi SDK source at that exact tag (`github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig/sig.go`, fetched live from `raw.githubusercontent.com/pulumi/pulumi/v3.245.0/...`) confirms the constants:
  ```go
  package sig
  const (
      Key    = "4dabf18193072939515e22adb298388d"
      Secret = "1b47061264138c4ac30d75fd1eb44270"
  )
  ```
  This matches the brief's cited signature pair exactly — confirming the brief's technical description is accurate and the import path `github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig` is real and already vendored at the pinned SDK version.
- `pkg/clouds/pulumi/aws/provider_test.go` exists (confirmed via code search) and is Gomega/`RegisterTestingT(t)`-style, as PM cited — good test-style model for the new file.

## 1. Commit + branch identity

No code mutations in this turn — Architect role, design only.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | Architect turn: design handoff only, no repo mutation |

**Planned files for Developer** (not yet created — these are the implementation target, not a claim of existing work):
- `pkg/clouds/pulumi/redact.go` — new, package `pulumi` (same package as `events.go`)
- `pkg/clouds/pulumi/redact_test.go` — new, same package, Gomega table-driven style matching `pkg/clouds/pulumi/aws/provider_test.go`'s convention
- `pkg/clouds/pulumi/events.go` — one-line edit: `procDiagnosticEvent` calls the new helper before logging

## 3. Design

### 3.1 Where the fix lives, and why not inside `pkg/api/logger/redact.go`

Two independent, layered redaction passes — not one function doing both jobs:

1. **New, targeted pass** in `pkg/clouds/pulumi/redact.go`, called from `procDiagnosticEvent` *before* the message reaches `p.logger.Info`. This is structural: it looks for Pulumi's secret-envelope shape (`sig.Key`/`sig.Secret` pair) and masks the sibling `value` field.
2. **Existing generic pass**, `logger.Redact()`, still runs unconditionally inside `logger.println` on the already-masked string — defense-in-depth for any PEM/private-key material that might independently appear in the same line (e.g. a GCP SA key embedded elsewhere in a diagnostic dump).

Rationale for NOT merging them: `logger.Redact()`'s detection technique is base64-decode-and-sniff for PEM bodies — a completely different parsing strategy from JSON-envelope structural matching. Conflating them in one function risks both false negatives (PEM sniffing doesn't know about `sig.Key`) and false positives (envelope matching doesn't know about PEM). Keeping them separate, composed in sequence, satisfies the brief's requirement ("the redaction should be a property of `procDiagnosticEvent` itself... not something each provider has to remember") while not disturbing the existing fleet convention.

### 3.2 `procDiagnosticEvent` — the one-line change

```go
func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
	p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s",
		p.contextActionName(ctx), evt.DiagnosticEvent.URN,
		redactSecretEnvelope(evt.DiagnosticEvent.Message))
}
```
(`redactSecretEnvelope` lives in the new file, same package — no import cycle, no cross-package reach into `pkg/api/logger`.)

### 3.3 `redactSecretEnvelope` — detection + masking strategy

`evt.DiagnosticEvent.Message` is a plain `string` (confirmed: `events.EngineEvent.DiagnosticEvent.Message` from `pulumi/sdk/v3/go/auto/events`) — not a structured `PropertyValue`. Pulumi's wire-format secret envelope, when it leaks into a free-text diagnostic dump, appears as a flat JSON object:
```json
{"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":"<secret-plaintext>"}
```
(this is the brief's own description of the leaked log shape, and matches Pulumi's documented secret-marshaling convention — `sig.Key`→`sig.Secret` plus a sibling `value` key).

Design of the function (scanner, not full JSON parse — matches the house style of `pkg/api/logger/redact.go`, which is also regex/substring-based rather than a JSON unmarshal):

1. Build the signature substring at init time from the imported constants rather than hardcoding the magic strings a second time:
   ```go
   import "github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig"
   var sigPair = fmt.Sprintf("%q:%q", sig.Key, sig.Secret) // `"4dabf181...":"1b470612..."`
   ```
2. Find each occurrence of `sigPair` in the message.
3. For each occurrence, scan outward for the enclosing JSON object: walk backward to the nearest unmatched `{`, forward to the matching `}` (simple brace-depth counter — the envelope objects observed in practice are flat, i.e. no nested braces, so a depth counter that stops at the first balanced `}` is sufficient and keeps the implementation small; this is a stated, non-blocking limitation for a secret value that is itself a nested JSON object/array — flagged as follow-up below, not required by this slice's DoD).
4. Within that extracted envelope substring, replace the JSON string value of the `"value"` key with a redaction marker (e.g. `"value":"[redacted]"`), via a regex scoped to *within the envelope substring only* (`"value"\s*:\s*"(?:[^"\\]|\\.)*"` → `"value":"[redacted]"`), so the replacement cannot accidentally hit an unrelated `"value"` key elsewhere in the message.
5. Splice the masked envelope back into the original message in place; repeat for every occurrence (the brief's incident shows the same envelope printed 4x in one message — the loop must handle repeats, not just the first match).
6. Return the modified message unchanged if no `sigPair` occurrence is found — zero behavioral change for the overwhelming majority of diagnostic messages (preserves AC "non-secret diagnostic content... remains visible").

Pseudocode shape for Developer:
```go
func redactSecretEnvelope(msg string) string {
	sigPair := fmt.Sprintf("%q:%q", sig.Key, sig.Secret)
	for {
		idx := strings.Index(msg, sigPair)
		if idx == -1 {
			return msg
		}
		start := lastUnmatchedBraceBefore(msg, idx)
		end := matchingBraceAfter(msg, idx)
		if start == -1 || end == -1 {
			return msg // malformed/unbounded — don't loop forever, bail safely
		}
		envelope := msg[start : end+1]
		masked := valueFieldRegex.ReplaceAllString(envelope, `"value":"[redacted]"`)
		msg = msg[:start] + masked + msg[end+1:]
		// advance search past the replaced region on the next loop iteration
	}
}
```
(Exact helper names/signatures are Developer's call; the invariant that must hold is: loop terminates, handles repeat occurrences, and never regresses to returning the original envelope unmasked.)

### 3.4 Unit test matrix (new `pkg/clouds/pulumi/redact_test.go`)

Following `pkg/api/logger/redact_test.go`'s rigor (table-driven, positive + negative controls, `RegisterTestingT(t)` Gomega style per `pkg/clouds/pulumi/aws/provider_test.go` convention):

| Test | Purpose | Fixture (synthetic only) |
|---|---|---|
| `TestDetectsPulumiSecretEnvelope` | Pure signature-detection test, independent of any provider payload shape | `{"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":"X"}` → asserts `"X"` masked, envelope recognized |
| `TestRedactSecretEnvelope_MasksSecretValue` | AC3 — secret-marked value does not survive | Representative fixture shaped like the real `getResource Invoke RPC finished` message, using fake AWS-shaped values `AKIAFAKEEXAMPLE1234` / `fakeSecretXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX`, repeated 4x in one message (per the incident's actual repeat count) — assert none of the 4 occurrences of either fake value appear in output |
| `TestRedactSecretEnvelope_PreservesNonSecretValues` | AC4 — negative control: non-secret content unaffected | A message with an un-enveloped `"value":"some-non-secret-string"` (no `sigPair` adjacent) plus ordinary diagnostic prose (resource URN, diff summary text) → assert byte-identical passthrough |
| `TestRedactSecretEnvelope_HandlesRepeatedEnvelopes` | Matches incident's "printed 4x in one log" detail | Same envelope JSON repeated 4 times in one string → all 4 masked, nothing else altered |
| `TestProcDiagnosticEvent_RedactsBeforeLogging` | AC1/AC3 integration — proves the chokepoint itself, not just the helper | Construct `events.EngineEvent{DiagnosticEvent: &events.DiagnosticEvent{URN: "urn:pulumi:...", Message: <fixture>}}`, call `p.procDiagnosticEvent`, capture stdout (reuse/adapt the `captureStdout` helper pattern from `pkg/api/logger/redact_test.go`), assert fake secret value absent from captured output and resource URN/action name still present |

All fixture values MUST be synthetic (`AKIAFAKEEXAMPLE1234`-style) per brief's secret-hygiene requirement — no real credential shape reused from the live incident.

### 3.5 Acceptance-criteria cross-check against this design

| Brief requirement | How this design satisfies it |
|---|---|
| Chokepoint, not per-provider | Single new function called from the one call site (`procDiagnosticEvent`) that *every* provider's `DiagnosticEvent` passes through (confirmed: `processEvent`'s `case evt.DiagnosticEvent != nil` is the sole dispatch for all clouds) — no AWS/GCP/K8s package touched |
| Detect Pulumi's own envelope markers | Built directly on `sig.Key`/`sig.Secret` imported from the vendored SDK, not re-hardcoded magic strings |
| Preserve non-secret diagnostic usefulness | Masking is scoped to the `value` field *inside* the matched envelope substring only; everything outside an envelope match (URNs, diff kinds, prose) passes through untouched |
| No real credential anywhere | All fixtures synthetic per §3.4 |

## 4. Known limitation / follow-up (not blocking S3, flag only)

- A secret value that is itself a nested JSON object/array (rather than a scalar string) inside the envelope will not be correctly brace-matched by the simple depth-counter described in §3.3 if it contains its own `{`/`}`. The brief's concrete incident (AWS access key + secret, both scalar strings) does not hit this case, and the DoD only requires that shape to be covered — but Developer/QA should note it as a residual gap. Recommend a follow-up roadmap item if nested secret values are ever observed in practice (`record_task` candidate, not scoped here).
- PM's NICE-TO-HAVE #6 (`diffSummary` / `procResourceOutputsEvent` / `procResourcePreEvent` paths also render `apitype.PropertyDiff` values and are a related but separate exposure surface) — reaffirming this is explicitly OUT of S3's scope per the brief's DoD naming `procDiagnosticEvent` specifically. Worth a separate roadmap slice, not expanding this dispatch.
- `pkg/clouds/pulumi/aws/provider.go:96-99`'s plain `sdk.String` args remain untouched per CUT #9 — confirmed not part of this design.

## 5. Tests run

N/A — Architect turn, no code written. Test matrix in §3.4 is the specification Developer must implement and run (`go test ./pkg/clouds/pulumi/...`).

## 6. Handoff guidance for Developer

- This is the **3rd dispatch attempt** of this exact slice; 2 prior Developer-character turns crashlooped (`exit status 143`, "claude emitted no output beyond init for 5m0s") before writing any code, both times after a clean Architect handoff (confirmed clean again this turn). If you hit the same hang at turn startup, report it plainly as the known infra pattern on this slice/repo combination rather than guessing at a workaround.
- Confine all new code to `pkg/clouds/pulumi/` (package `pulumi`) plus the one-line call-site edit in `events.go` — do not touch any provider subpackage.
- Side-observation (not your scope to fix): this run targets `api`, whose branch protection requires 2 human reviews, blocking SDLC auto-merge. Note your PR number + `mergeable_state` in your own handoff as an observational data point for the `api-repo-branch-protection-blocks-sdlc-automerge` roadmap item — do not expand scope to address it.

**Verdict:** signoff