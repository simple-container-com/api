[thinking] # PM Handoff — `aws-provider-key-in-deploy-log` (redact.go AWS key patterns)

## 0. Context restated

A failed isolated deploy printed a live AWS access-key id **and** its secret access key in plaintext (4× in one log). Root cause: `pkg/api/logger/redact.go`'s `Redact()` function already sits behind every `Info()` log call (confirmed via `pkg/api/logger/logger.go:69` and `pkg/clouds/pulumi/provisioner.go:26` in a prior run — not re-verified by me, but the brief's reasoning is internally consistent with the file I read) and already strips private-key material via four regexes (`privateKeyBlock`, `privateKeyTail`, `privateKeyID`, `base64Run`→`holdsPrivateKey`). It has **zero** coverage for AWS credential shapes. `events.go` is correctly out of scope — it's a diagnostic-forwarding path that already inherits redaction from the logger it calls into; touching it would be scope creep.

I read `pkg/api/logger/redact.go` and `pkg/api/logger/redact_test.go` directly (both current on `main`) to ground this plan — not relying on the triggering run's prior claims.

## 1. Commit + branch identity

No code mutations in this turn. PM does not write code.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | (none — planning only) |

## 3. Acceptance criteria (restated + made concrete for Architect/Developer)

| AC# | Criterion | Notes for implementer |
|---|---|---|
| AC1 | Add an `awsAccessKeyID` regex matching `AKIA[0-9A-Z]{16}` (20 chars total) anywhere in a log message, redacted the same way `privateKeyID` is (replace with `redacted` constant, keep surrounding text) | Low false-positive risk — `AKIA` prefix is a fixed, real AWS convention. Safe to match unscoped. |
| AC2 | Add an `awsSecretAccessKey` regex matching a 40-char AWS secret key **(MUST be context-anchored to a recognizable label, e.g. `secret_access_key`/`SecretAccessKey`/`aws_secret_access_key`/`Secret Access Key`, case-insensitive, as a prefix capture group)** — follow the `privateKeyID` precedent (`(private_key_id...)[0-9a-f]{40}` → capture-group-1 + redacted) rather than an unscoped 40-char base64 match | **Hard constraint, verified by me against the current test file**: `redact_test.go`'s `TestRedactLeavesOtherTextAlone` already asserts `"commit 0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"` (a 40-char hex string) passes through **unchanged**. An unscoped "40 alnum/+/= chars" regex would match that hex run (hex is a subset of base64 charset) and **break an existing passing test** — i.e. silently increase blast radius and regress CI. The label-anchor avoids this exactly as `privateKeyID` already does for service-account key ids. Architect: make this anchoring decision explicit in the design doc; Developer: do not submit an unscoped variant. |
| AC3 | `redact_test.go` gets new table cases: (a) bare `AKIA...` id redacted in isolation and inside a Pulumi-diff-style line; (b) labeled secret key (`secret_access_key: "<40char>"`, `AWS_SECRET_ACCESS_KEY=<40char>` env-style, and a Pulumi diff line) redacted; (c) a negative case proving adjacent/look-alike text is NOT touched — reuse the existing `commit <40-hex>` case as a regression guard, plus a new one for a 20-char non-`AKIA`-prefixed string that looks access-key-ish but isn't | Follow the existing table-driven style (`TestRedactRemovesOtherShapes` / `TestRedactLeavesOtherTextAlone` / `TestRedactKeepsTheTextAroundKeys`) rather than inventing a new test structure. |
| AC4 | `events.go`, `provisioner.go`, key-rotation/revocation logic are **untouched** | Enforced by review — Developer's diff must show zero hunks outside `pkg/api/logger/redact.go` and `pkg/api/logger/redact_test.go`. |
| AC5 | Gate is `make check` (go vet, ~2.3GB peak, proven safe) **and** `go test ./pkg/api/logger/...` only — never a broader build/test | Per root `CLAUDE.md`: `make build`/`make build-all`/bare `go build ./...`/`go test ./...` link `cmd/sc` and peak 12.9–14.2GB → OOM in this 12GB swapless microVM. This is the 5th dispatch attempt on this slice after 4 OOM deaths; Developer must not deviate from this gate. |

## 4. Scope decisions (MoSCoW)

| Item | Label | Rationale |
|---|---|---|
| `AKIA...` access-key-id pattern in `redact.go` | **MUST** | Directly named in brief, concrete real-world shape, low FP risk |
| Labeled 40-char AWS secret-key pattern in `redact.go` | **MUST** | Directly named in brief; the actual secret value is the higher-severity leak |
| Context-anchoring the secret-key regex to a label | **MUST** | Not explicitly spelled out in the brief but required to avoid regressing the existing `commit <sha>` test and to avoid a flood of false positives on arbitrary 40-char hex/base64 strings (commit SHAs, digests, etc.) elsewhere in logs |
| New table test cases in `redact_test.go` | **MUST** | Explicit AC in brief |
| Unscoped/bare 40-char secret-key matching (no label) | **CUT** | Would break existing passing test and balloon false-positive rate — rejected design, documented above so nobody re-derives it |
| Changes to `events.go` / `provisioner.go` | **CUT** | Explicitly out of scope per brief; already-redacted chokepoint |
| Key rotation/revocation (S1/S2) | **CUT** | Explicitly operator-owned per brief |
| Broader AWS secret shapes (session tokens, STS creds) | **NICE-TO-HAVE / CUT for this slice** | Not named in brief; flag as a follow-up if operator wants wider AWS coverage later — do not gold-plate this slice |
| Running `make build`/`make build-all`/bare `go build|test ./...` | **CUT (forbidden)** | Will OOM per CLAUDE.md; 4 prior attempts died this way |

## 5. Handoff guidance for Architect

- Single-file change: `pkg/api/logger/redact.go` (+ its test file). No interface/API changes, no new deps.
- Design the two new regexes to slot into the existing `var (...)` block and the existing `Redact()` function body, using the same replace-with-`redacted`-constant pattern already used for `privateKeyID` (capture group 1 + `redacted`, so surrounding text like the label itself is preserved — matches the "redact value, keep label" UX already established).
- Decide the exact label-anchor wording/case-insensitivity for the secret-key regex (e.g. `(?i)(secret[_ ]?access[_ ]?key\s*[:=]\s*"?)[A-Za-z0-9+/]{40}`) — this is the one open technical judgment call, flag it in the design doc for a 1-line review rather than a back-and-forth.
- Verify the new secret-key regex does NOT match the existing `commit <40-hex>` negative-test string before considering the design closed — that's a concrete, cheap regression check Architect/Developer can run immediately.
- Gate: `make check` then `go test ./pkg/api/logger/...`. Do not broaden.

## 6. Tests run

None — PM turn, no code touched.

**Empty-PR-OK:** yes — this is a planning-only PM turn with no source changes; the real-file verification (CLAUDE.md, redact.go, redact_test.go, preview_redact_test.go) was done via read-only `read_repository_file` calls to ground the plan, not to mutate anything.

**Verdict:** signoff