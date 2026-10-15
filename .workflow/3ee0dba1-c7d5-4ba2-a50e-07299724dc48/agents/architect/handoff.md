[thinking] # Architect Handoff — `aws-provider-key-in-deploy-log` (S3: redact diagnostic-event forwarding)

`slice_id: aws-provider-key-in-deploy-log`

## 0. Context recap (re-dispatch)

Re-derived from scratch per the brief (prior Architect artifact from the crashed run `b0de022e` is not assumed to exist). I read the actual current files on `main` this turn — not from memory — and confirm the PM's repo-level finding:

| File | Blob SHA (as read this turn) | Relevant fact |
|---|---|---|
| `pkg/clouds/pulumi/events.go` | `00aedc4d42601d82cb031435560d40b41e8843d9` | `procDiagnosticEvent` (line ~75) calls `p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)` |
| `pkg/api/logger/logger.go` | `5996c09c358ed540b882d589cd52cbe780de4d88` | `(l logger) println` does `fmt.Println(Redact(fmt.Sprintf(...) + fmt.Sprintf(format, a...)))` — every `Info/Warn/Error/Debug` call on every `Logger` instance in the repo passes through `Redact()`. This is confirmed, not assumed. |
| `pkg/api/logger/redact.go` | `551c5280489c64a23c0e33a4c85aba374ea9cc83` | `Redact()` currently handles only PEM/PGP private-key blocks, `private_key_id`, and base64 runs ≥100 chars that decode to `"PRIVATE KEY"`. It has **zero** notion of Pulumi's secret envelope and would not touch a short AWS access-key/secret pair (too short for the base64 floor, not PEM-shaped). |
| `pkg/api/logger/redact_test.go` | `d72c464d7a5f3ca05e510b474831e28060d8ceb0` | Established test pattern: table-driven shape tests (`TestRedactRemovesOtherShapes`), a negative test (`TestRedactLeavesOtherTextAlone`), and an end-to-end logger test (`TestLoggerRedactsWhatItPrints` using `captureStdout`). |
| `pkg/clouds/pulumi/aws/provider.go` | `28e039e7eafa782f49b3632c1f77760ce073335a` | Confirmed out of scope — `applyAWSProviderCreds` passes `sdk.String(accessKey)`/`sdk.String(secretAccessKey)` unmarked; this is S1-adjacent hardening explicitly excluded by the brief. |

Pulumi's secret-envelope signature constants, verified this turn against upstream `pulumi/pulumi/sdk/go/common/resource/sig/sig.go` (master):
```
Key    = "4dabf18193072939515e22adb298388d"
Secret = "1b47061264138c4ac30d75fd1eb44270"
```
and confirmed in `pulumi/pulumi/sdk/go/common/resource/plugin/rpc.go` (`MarshalPropertyValue`, `IsSecret` branch): a secret serializes as
```go
resource.PropertyMap{
    resource.SigKey: resource.NewProperty(resource.SecretSig),
    "value":         v.SecretValue().Element,
}
```
— i.e. an object carrying the two signatures plus a `"value"` field holding the plaintext element, with **no constraint on the plaintext's length or shape**. `PropertyMap.StableKeys()` sorts alphabetically, so the sig key (`"4dabf1…"`, starts with a digit) sorts **before** `"value"` in any serialization that honours stable key order — useful for an anchor-then-look-forward redaction strategy (below).

**I could not confirm the exact textual rendering of the leaked `getResource Invoke RPC finished` diagnostic line** (whether it's JSON-ish `"value":"..."` or a protobuf debug-text rendering like `value:{string_value:"..."}`) — that string is built inside the Pulumi engine/CLI binary, outside this repo, and I have no access to a real captured sample (only the incident's prose description). Flagging this explicitly rather than guessing a false-confident format: **Developer must treat both plausible renderings as must-cover, and write test fixtures that document which shape each test models — not claim either is the verbatim incident payload.**

## 1. Architectural decision: fix point

**Decision: extend `Redact()` in `pkg/api/logger/redact.go`. Do NOT add a wrapper/change in `events.go`.**

Why, in order of weight:

1. **It's already the one chokepoint everything converges on.** `logger.println` calls `Redact()` on every `Info/Warn/Error/Debug` line — `procDiagnosticEvent` already flows through it today, unmodified. There is nothing to change in `events.go` at all.
2. **It protects every other event handler for free.** `procResourceOutputsEvent`, `procResourcePreEvent`, `procPolicyEvent`, and the `evt.Error != nil` branch all call `p.logger.*` too — a fix scoped to `events.go:75` alone would leave those exposed to the exact same envelope shape (e.g. a resource diff that includes a secret property). Extending `Redact()` closes all of them with one change — this is what "chokepoint, not per-provider" means taken to its logical conclusion, beyond the brief's literal line pointer.
3. **It needs zero new provider-marking discipline.** AWS, GCP, Yandex, and any future provider's diagnostics/output/preview events all land in this same pipe — no provider code changes, matching AC "works for AWS, GCP, Yandex, any future provider without each one marking inputs."
4. **It follows the package's own established, tested idiom** (compiled package-level `regexp.MustCompile` vars + `Redact()` applying each pattern in sequence + a `redact_test.go` table of positive/negative shapes) rather than inventing a second redaction mechanism living in `pkg/clouds/pulumi`.
5. **No new dependency.** `pkg/api/logger` stays a leaf package — hardcode the two sig *string constants* with a comment citing their public origin (`github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig`), do not import Pulumi SDK packages into the logger.

This satisfies the brief's literal pointer (`events.go:75`'s output *is* fixed — transitively, by fixing what it's piped through) without the narrower, incomplete read of "patch only that one call site."

## 2. Design: the new redaction pattern

Add to `pkg/api/logger/redact.go`, following the file's existing structure (doc comment explaining *why*, then pattern, then wiring into `Redact()`):

```go
const (
    // pulumiSecretSigKey / pulumiSecretSig are Pulumi's own signature
    // constants for a secret-wrapped property on the wire (verified against
    // github.com/pulumi/pulumi/sdk/v3/go/common/resource/sig). A secret
    // value serializes as an object carrying BOTH signatures plus a "value"
    // field holding the plaintext, in whichever textual form the layer that
    // logs it chooses to render an object (JSON-like `"value":"..."`, or a
    // protobuf debug/text rendering like `value:{string_value:"..."}`).
    // This is Pulumi's OWN envelope, independent of what's inside it — an
    // AWS access key, a GCP service-account token, anything a provider
    // marks secret — so it is the one thing that lets us catch the leak
    // regardless of provider or value shape, unlike the shape-based
    // patterns above which only recognize private-key material.
    pulumiSecretSigKey = `4dabf18193072939515e22adb298388d`
    pulumiSecretSig     = `1b47061264138c4ac30d75fd1eb44270`
)

var (
    // pulumiSecretEnvelopeAnchor finds the two signatures close together —
    // StableKeys() sorts alphabetically, so the sig key (starts with a
    // digit) always precedes the secret value when both render. A bounded
    // gap tolerates whatever punctuation/field-name wrapping the renderer
    // uses between them.
    pulumiSecretEnvelopeAnchor = regexp.MustCompile(pulumiSecretSigKey + `.{0,80}?` + pulumiSecretSig)

    // pulumiSecretEnvelopeValue, applied starting right after the anchor,
    // captures the "value" field's plaintext in either rendering.
    pulumiSecretEnvelopeValue = regexp.MustCompile(`(?s)\A.{0,120}?"?value"?\s*[:=]\s*\{?\s*(?:string_value\s*:\s*)?\\*"([^"\\]*)`)
)
```

Wire into `Redact()` (after the existing base64Run pass): scan for each `pulumiSecretEnvelopeAnchor` match, then run `pulumiSecretEnvelopeValue` against the remaining tail of the string from the anchor's end, and replace the captured group-1 span with `redacted` — an anchor-then-look-forward two-step, not one giant regex, because the exact separator syntax between the sig pair and the `"value"` field is the part I could not confirm (§0). This mirrors the file's own `privateKeyTail`/`privateKeyBlock` split-pattern style.

**Explicit non-goals** (carried from PM's scope, confirmed sound):
- A `"value"` field that is itself a nested object/array (a secret resource reference) is a documented non-goal — the realistic incident shape is a flat string (AWS secret key), which is what tests must cover.
- This does not attempt to parse/validate JSON or protobuf structurally — same "regex over shape" philosophy as the rest of `redact.go`.

## 3. Files to touch (for Developer)

| Path | Change | Why |
|---|---|---|
| `pkg/api/logger/redact.go` | Add `pulumiSecretEnvelopeAnchor`/`pulumiSecretEnvelopeValue` + wire into `Redact()` | The chokepoint fix itself |
| `pkg/api/logger/redact_test.go` | Extend with: (a) envelope-present → redacted, using a synthetic fake-shaped AWS key (e.g. `AKIAFAKEFAKEFAKEFAKE` style, never real), modeled on both the JSON-ish and protobuf-text-ish renderings described in §0/§2, each test clearly commented as "representative, not verbatim"; (b) envelope-absent → byte-for-byte unchanged (extend `TestRedactLeavesOtherTextAlone`); (c) a realistic `getResource Invoke RPC finished`-shaped synthetic sample (tok + args/response wrapping the envelope) | AC1–AC3 of the brief |
| `pkg/clouds/pulumi/events_test.go` (**new file** — confirmed via `search_repository_code` that no `events_test.go` exists in this package today) | New end-to-end test: construct `p := &pulumi{logger: logger.New()}` (same pattern as `refresh_test.go`/`refresh_integration_test.go`, confirmed real), build an `events.EngineEvent{DiagnosticEvent: &events.DiagnosticEvent{URN: ..., Message: <synthetic envelope-carrying message>}}`, call `p.procDiagnosticEvent(ctx, evt)` with stdout captured (Developer writes a local `os.Pipe`-based capture helper — `pkg/api/logger`'s `captureStdout` is unexported and in a different package, so it cannot be reused directly), assert the fake secret text never appears in the captured output. This proves the chokepoint end-to-end, not just `Redact()` in isolation — it would catch a future regression where the pulumi package stops routing through `logger.Logger`. | Closes the "chokepoint, not per-provider" AC at the actual call site named in the brief, without modifying that call site |
| `pkg/clouds/pulumi/events.go` | **No change** | Already routes through the fixed chokepoint |
| `pkg/clouds/pulumi/aws/provider.go` | **No change** | Confirmed out of scope (S1 done, S2 is an operator call) |

## 4. Acceptance criteria verification

I made no code changes this turn (Architect/design turn) — the table below reflects what I **design-confirmed by reading the real repo**, not test execution. Build/vet/test execution is Developer's responsibility next.

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Diagnostic message with Pulumi's secret envelope is redacted regardless of provider | DESIGN-CONFIRMED (not yet implemented) | Chokepoint confirmed live: `logger.go:println` → `Redact()` on every log line (read verbatim, SHA `5996c09c…`); `events.go:procDiagnosticEvent` confirmed to call `p.logger.Info` (SHA `00aedc4d…`). Sig constants verified against upstream `pulumi/pulumi` master. Implementation itself is Developer's next step — see §2/§3. |
| AC2 | Message with no envelope passes through byte-for-byte unchanged | PENDING (Developer) | Requires running the extended `redact_test.go`; design intent stated in §2 (anchor-based, so no-match ⇒ no replacement, following the file's existing negative-test discipline). |
| AC3 | Realistic `getResource Invoke RPC finished`-family sample is redacted | PENDING (Developer) — **flagged uncertainty**: exact wire rendering not confirmable from this repo (§0); test fixtures must be built as "representative," per brief's own instruction, and Developer should validate the anchor+value regex against BOTH plausible renderings (JSON-ish, protobuf-text-ish) rather than one guessed shape. |
| AC4 | Fix is chokepoint-level; works for AWS/GCP/Yandex/future providers without per-provider marking | DESIGN-CONFIRMED | `provider.go` untouched (confirmed by design — no change listed in §3); every provider's events already funnel through `pulumi.logger` → `Redact()`, confirmed by reading `events.go` and the `type pulumi struct{ logger logger.Logger }` / `InitPulumiProvisioner` wiring (`provisioner.go`, confirmed via `search_repository_code`). |
| AC5 | Full test suite for touched packages passes | PENDING (Developer) — run `go test ./pkg/api/logger/... ./pkg/clouds/pulumi/...` after implementing §2/§3. |
| AC6 | `go build ./...` and `go vet ./...` succeed | PENDING (Developer). |
| AC7 | No real/live credential value anywhere in code/tests/commits/handoffs | PASS (this handoff) | This document contains no real credential values — only the two public Pulumi signature constants (not secrets) and references to synthetic fake-shaped fixtures Developer must create (e.g. `AKIAFAKEFAKEFAKEFAKE`-style), never real keys. Self-scanned before emitting: no ≥20-char random-looking string present that isn't a known public identifier (the two sig constants are public Pulumi source constants, cited with their origin). |

## 5. Tests run

Not applicable — no code mutated this turn. Verification performed instead:
- `read_repository_file` on `pkg/clouds/pulumi/events.go`, `pkg/clouds/pulumi/aws/provider.go`, `pkg/api/logger/redact.go`, `pkg/api/logger/redact_test.go`, `pkg/api/logger/logger.go`, `pkg/api/logger/logger_test.go` (all on `api`'s default branch) — confirmed the chokepoint and the absence of secret-envelope handling, with blob SHAs captured in §0.
- `search_repository_code` confirming no `events_test.go` exists yet in `pkg/clouds/pulumi`, and confirming the `&pulumi{logger: logger.New()}` test-construction pattern already used in `refresh_test.go` / `refresh_integration_test.go`.
- `list_repository_commits` on `events.go` and `redact.go` — most recent real commit touching `redact.go` is `dc6a461f5e58814782f201530eb6d04d136be6a0` ("fix(logging): redact private key material from all log output (#429)", 2026-10-09) — this is the PRIOR redaction work being extended, not a commit of mine.
- `fetch_url` against `raw.githubusercontent.com/pulumi/pulumi/master/sdk/go/common/resource/sig/sig.go` and `.../plugin/rpc.go` — confirmed the two signature constants and the `MarshalPropertyValue` secret-envelope shape (`SigKey` + `"value"` field) verbatim from upstream source.

## 6. Handoff guidance for Developer

- Implement §2 in `pkg/api/logger/redact.go`, following the file's existing comment/pattern style exactly (it's dense but consistent — read `privateKeyTail`/`privateKeyBlock` as your template for the "anchor, then bounded look-forward" idiom).
- Write `redact_test.go` fixtures that explicitly **label which rendering they model** (JSON-ish vs protobuf-text-ish) — do not claim either is the verbatim leaked payload; the brief itself calls for "representative," not exact reproduction.
- `events_test.go` is a genuinely new file — there's no prior version to extend.
- If, once you implement and test both envelope renderings, the anchor-then-look-forward regex in §2 proves too brittle for the actual shape you settle on, that's an acceptable implementation deviation — the binding requirement is AC1–AC4, not my specific regex syntax. Document whatever final shape you land on in your handoff with the real test output.
- Scope `go test` to `./pkg/api/logger/... ./pkg/clouds/pulumi/...` per the turn-size guidance — this is a large repo, don't run full `./...`.
- Secret hygiene: every fixture you write must be a synthetic fake-shaped value (e.g. `AKIAFAKEFAKEFAKEFAKE` or an obviously-fake 40-char hex string) — never a real credential, never anything resembling the actual leaked key from the incident.

**Empty-PR-OK:** yes — this is an Architect design turn with zero source-code mutation by design; the AC table above is design-level confirmation from real repository reads (file SHAs, commit history, upstream Pulumi source), not a narrative claim of tests that weren't run.

**Verdict:** signoff