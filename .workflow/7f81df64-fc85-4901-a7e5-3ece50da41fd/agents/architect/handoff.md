[thinking] # Architect Handoff — S3: Redact Pulumi secret-envelope values in `procDiagnosticEvent`

## 0. Repo verification performed this turn (read directly, current HEAD of `simple-container-com/api`)

- `pkg/clouds/pulumi/events.go` — read in full. Confirmed `sha 00aedc4d42601d82cb031435560d40b41e8843d9`, size 3450 bytes. `procDiagnosticEvent` is exactly as PM reported:
  ```go
  func (p *pulumi) procDiagnosticEvent(ctx context.Context, evt events.EngineEvent) {
      p.logger.Info(ctx, "[%s][pulumi/diagnostic] %s: %s", p.contextActionName(ctx), evt.DiagnosticEvent.URN, evt.DiagnosticEvent.Message)
  }
  ```
- `pkg/api/logger/redact.go` — read in full. Confirmed `sha 551c5280489c64a23c0e33a4c85aba374ea9cc83`, size 3629 bytes. Confirmed `logger.go`'s `println()` ALREADY calls `Redact()` unconditionally on the fully-formatted string for every `Info`/`Warn`/`Error`/`Debug` call: `fmt.Println(Redact(fmt.Sprintf("%s%s: ", datePrefix, levelString) + fmt.Sprintf(format, a...)))`. This is a load-bearing fact for the design decision below — `procDiagnosticEvent`'s call to `p.logger.Info` already funnels through `Redact()` today, it just isn't taught to recognize Pulumi's secret envelope yet.
- `pkg/clouds/pulumi/preview_redact_test.go` — read in full, confirmed existing test precedent/style.
- Fetched Pulumi SDK v3.245.0 source directly from GitHub (`pulumi/pulumi@v3.245.0`) to pin the exact wire shape rather than guess it:
  - `sdk/go/common/resource/sig/sig.go` → confirms `sig.Key = "4dabf18193072939515e22adb298388d"`, `sig.Secret = "1b47061264138c4ac30d75fd1eb44270"` (matches PM's digest).
  - `sdk/go/common/resource/plugin/rpc.go`, function `MarshalPropertyValue` → confirms EXACTLY how a `resource.MakeSecret(...)` property is marshaled when `opts.KeepSecrets == true`:
    ```go
    secret := resource.NewProperty(resource.PropertyMap{
        resource.SigKey: resource.NewProperty(resource.SecretSig),
        "value":         v.SecretValue().Element,
    })
    return MarshalPropertyValue(key, secret, opts)
    ```
    i.e. a 2-key object: `{"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":<plaintext>}`. `PropertyMap.StableKeys()` sorts keys lexically, so when this is re-serialized through `MarshalProperties` the sig key (`"4...` starts with a digit) sorts before `"value"` — but I'm not assuming the diagnostic-event code path necessarily goes through `StableKeys()`, so the design below is order-tolerant in both directions (see §2).
  - Could not fetch the exact Pulumi **engine** code path that assembles `DiagnosticEvent.Message` from a `getResource` gRPC response (file paths I tried under `pkg/display/` 404'd via raw.githubusercontent — likely renamed/relocated in this tag, and GitHub code search timed out). **This is a real gap**: I have ground-truth on the SECRET ENVELOPE SHAPE (confirmed from SDK source) but not 100%-confirmed ground-truth on exactly how/whether it's embedded verbatim vs. re-escaped inside `DiagnosticEvent.Message` text. I'm resolving this by directing Developer to derive the exact byte-for-byte shape from a real call into the SDK's own marshaling function inside the unit test (see §3) rather than hand-typing a guessed JSON string — this makes the test self-grounding regardless of which exact escaping the real incident text had.

## 1. Design decision

**Where the fix lives:** extend `pkg/api/logger/redact.go`'s `Redact()` — NOT a new pulumi-package-local helper.

Rationale (resolving PM's open design question, §3 of PM handoff):
- `logger.println()` already calls `Redact()` on every formatted log line, and `procDiagnosticEvent` calls `p.logger.Info` directly — so `Redact()` is "something `procDiagnosticEvent` calls" (AC2 is satisfied transitively, same as the *existing* PEM-redaction protection already covers `procDiagnosticEvent` today for key material, just not for Pulumi's secret envelope shape).
- Zero new call sites, zero new abstraction. This is the same additive-regex idiom the file already uses for PEM blocks — I'm resisting the urge to build a structural JSON-walker (balanced-brace scanner + `json.Unmarshal` of embedded fragments) for this slice: the envelope is a fixed 2-key shape with two literal, protocol-level constant markers, so a tolerant regex is sufficient and matches the file's established style. A structural parser would be the over-engineered answer here — defer it unless a future incident shows nested/multi-valued envelopes regex can't handle.
- It is provider-agnostic by construction — it keys off Pulumi's own `sig.Key`/`sig.Secret` constants, not AWS-shaped strings, which is the brief's explicit generality requirement and does NOT touch `pkg/clouds/pulumi/aws/provider.go` (out of scope).
- Side-effect check: the new regexes only fire on the literal 32-hex-char sig constants, which cannot appear in ordinary diagnostic text (resource names, URNs, "Resources: N to update") — no regression risk to AC5, and it composes cleanly with the existing PEM regexes already in `Redact()`.

## 2. Concrete change — `pkg/api/logger/redact.go`

Add two new package-level regexes (order-tolerant: Pulumi's secret envelope's two keys can appear sig-key-first or `"value"`-first depending on serialization path; quotes may carry escaping if the message passed through another JSON/log layer, same tolerance idiom the file already uses for `privateKeyID`):

```go
// A Pulumi "secret envelope" — the sig key/value pair Pulumi's own SDK stamps onto
// any resource.MakeSecret()-wrapped property when serializing it with KeepSecrets
// (see pulumi/sdk/v3/go/common/resource/plugin.MarshalPropertyValue, pinned SDK
// v3.245.0). A diagnostic event carrying a provider's raw getResource response
// embeds this shape verbatim for every secret-marked input/output, regardless of
// which provider (AWS, GCP, Yandex, ...) marked it — so matching on it, rather than
// on any provider-specific credential shape, is what makes this redaction generic.
secretEnvelopeKeyFirst = regexp.MustCompile(`\\*"4dabf18193072939515e22adb298388d\\*"\s*:\s*\\*"1b47061264138c4ac30d75fd1eb44270\\*"\s*,\s*\\*"value\\*"\s*:\s*\\*"((?:[^"\\]|\\.)*)\\*"`)
secretEnvelopeValueFirst = regexp.MustCompile(`\\*"value\\*"\s*:\s*\\*"((?:[^"\\]|\\.)*)\\*"\s*,\s*\\*"4dabf18193072939515e22adb298388d\\*"\s*:\s*\\*"1b47061264138c4ac30d75fd1eb44270\\*"`)
```

In `Redact()`, add before the final `base64Run` pass:

```go
s = secretEnvelopeKeyFirst.ReplaceAllString(s, `"4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270","value":"`+redacted+`"`)
s = secretEnvelopeValueFirst.ReplaceAllString(s, `"value":"`+redacted+`","4dabf18193072939515e22adb298388d":"1b47061264138c4ac30d75fd1eb44270"`)
```

No changes needed to `pkg/clouds/pulumi/events.go` itself — `procDiagnosticEvent` is unmodified; the fix is entirely inside the shared redaction pipeline it already calls into.

**Known limitation to document in the PR / code comment (not a blocker for this slice):** this regex redacts the envelope only when `"value"` is a JSON string. A secret wrapping a non-string (nested object/array) PropertyValue would not be caught by this pattern. The brief's incident (AWS access key + secret key) and both required unit tests are string-valued, so this is in-scope-complete; flag as a documented edge case for a future slice if it recurs with non-string secrets.

## 3. Test design — directed at Developer, grounded in the real SDK (not hand-typed JSON)

**Test A (AC3 — secret-envelope message gets redacted):** Don't hand-write the JSON fixture — derive it from the real SDK so the test is grounded in actual wire output, not a guess:
```go
import (
    "github.com/pulumi/pulumi/sdk/v3/go/common/resource"
    "github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
)
secretVal := resource.MakeSecret(resource.NewStringProperty("AKIAABCDEFGHIJKLMNOP"))
marshaled, err := plugin.MarshalPropertyValue("accessKey", secretVal, plugin.MarshalOptions{KeepSecrets: true})
// marshaled is *structpb.Value — render it to text the same way the diagnostic
// path would (protojson.Marshal(marshaled) is the most defensible choice: it's
// the standard way structpb values become JSON text in this codebase's deps).
// Embed that real text into a synthetic DiagnosticEvent.Message, call Redact(),
// and assert:
//   1. The literal secret "AKIAABCDEFGHIJKLMNOP" does NOT appear anywhere in the output.
//   2. The sig-key marker "4dabf18193072939515e22adb298388d" DOES still appear (envelope shape preserved, proving targeted not blanket redaction).
//   3. Surrounding non-secret text (e.g. a URN string placed before/after) is untouched.
```
If `protojson.Marshal`'s actual escaping differs from what the two regexes above tolerate (e.g. adds a space after `:`), **that's exactly why this is Developer's job to run first and adjust the regex whitespace tolerance (`\s*` is already permissive) against observed real output — not mine to guess further.**

**Test B (AC4 — regression guard):** Take a real non-secret diagnostic line (e.g. `"Resources: 1 to update"` or a plain URN diff line, following `preview_redact_test.go`'s style) through `Redact()` and assert byte-for-byte equality with the input.

**Test C (AC5, can combine with A/B):** Assert ordinary resource names/URNs in the SAME message as a redacted secret are preserved — i.e. only the envelope's `value` field is touched, nothing else in the line.

Both tests belong in `pkg/api/logger/redact_test.go` (same file/style as existing `TestRedactLeavesOtherTextAlone`) since the fix itself lives in that package, per design decision in §1.

## 4. Commands for Developer to run (AC6)
```
go build ./pkg/clouds/pulumi/... ./pkg/api/logger/...
go test ./pkg/clouds/pulumi/... ./pkg/api/logger/...
```

## 5. Operational notes carried forward from PM (unchanged, still true)
- Expect `mergeable_state: blocked` on the resulting PR against `api` (branch protection, 2 human reviews required) — normal, do not attempt to fix, just report PR number + state if a PR opens (AC7).
- **Developer executor stability flag (not a design/scope issue):** 3 prior dispatches on this exact slice died `exit status 143` in the Developer character's executor turn, after a clean Architect signoff all 3 times (this is attempt 4). The design above is unchanged in substance from what was clean on those prior passes — if you are the Developer and see unusual turn-startup behavior (hung MCP handshake, slow clone), that's the known pattern, not a sign this design is wrong.

## 6. Files touched

No code mutations in this turn — this is an architecture/design handoff only.

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 7. Acceptance criteria verification (design-level; Developer/QA execute)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | No credential material in deploy log for any provider's secret-marked properties, even via diagnostic event | DESIGN-READY | Regex keys off Pulumi's own `sig.Key`/`sig.Secret` constants (confirmed from SDK v3.245.0 source, §0), not any provider-specific shape — generic by construction |
| AC2 | Redaction lives in `procDiagnosticEvent` itself or something it calls | DESIGN-READY | `procDiagnosticEvent` → `p.logger.Info` → `logger.println` → `Redact()` (confirmed call chain by reading `logger.go`, §0) — no new call site added, reusing the existing chokepoint every log line already passes through |
| AC3 | Secret-envelope message → redacted (unit test) | PENDING DEV | Test design in §3 Test A — directs Developer to derive real wire text from `plugin.MarshalPropertyValue` rather than a guessed fixture |
| AC4 | Non-secret message → unchanged byte-for-byte (unit test) | PENDING DEV | Test design in §3 Test B, reuses `preview_redact_test.go` idiom |
| AC5 | Ordinary resource names/URNs untouched | PENDING DEV | Test design in §3 Test C |
| AC6 | `go build`/`go test` clean on `pkg/clouds/pulumi/...`, `pkg/api/logger/...` | PENDING DEV | Commands specified in §4; not run this turn (no code change made) |
| AC7 | PR number + `mergeable_state` reported if PR opens | PENDING DEV/DEVOPS | Carried forward, unchanged |

## 8. Tests run

None — this is a design-only turn, no code was changed. Test *specifications* for Developer are in §3, grounded in real SDK source read this turn (§0), not guessed.

**Empty-PR-OK:** yes — this is an Architect design turn with no source changes; design output is the handoff document itself, consistent with the PM→Architect transition for this slice.

**Verdict:** signoff