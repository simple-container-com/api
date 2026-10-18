[thinking] # Architect Handoff — `aws-provider-key-in-deploy-log` (redact.go AWS key patterns)

## 0. Context grounding (read fresh, this turn)

I re-read the current `main` state of both files directly (not relying on PM's prior read or the triggering run's claims):

- `pkg/api/logger/redact.go` (sha `551c5280489c64a23c0e33a4c85aba374ea9cc83`) — confirmed the `var (...)` block holds exactly 4 regexes (`privateKeyBlock`, `privateKeyTail`, `privateKeyID`, `base64Run`), the `redacted = "[redacted]"` constant, and `Redact()`'s linear `ReplaceAllString` chain ending in the `base64Run` callback. Zero AWS-shape coverage exists today.
- `pkg/api/logger/redact_test.go` (sha `d72c464d7a5f3ca05e510b474831e28060d8ceb0`) — confirmed `TestRedactLeavesOtherTextAlone` asserts `"commit 0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"` (40-char hex) passes through **unchanged** — this is the regression guard PM flagged; any unscoped 40-char-charset secret-key pattern would break it since hex ⊂ base64-url-safe charset.
- `pkg/clouds/pulumi/preview_redact_test.go` (sha `333453d10ab4bf277aee983b3356748e76b53548`) — confirms the existing table-test precedent style PM referenced (construct a real key, run it through `Redact`/`toPreviewResult`, assert fragments don't survive).
- `CLAUDE.md` (sha `626ed920ef80bed5631ddb3fc8707d3b224790f2`) — confirms the memory gate: `make check` (go vet, 2.3GB measured peak) is the only safe full-repo command; `make build`/`make build-all`/bare `go build|test ./...` all link `cmd/sc` and peak 12.9–14.2GB. Developer must scope to `go test ./pkg/api/logger/...` for the test gate, never `./...`.
- AWS IAM docs (docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html, fetched this turn) confirm the canonical shape I'm designing against: access-key-id example `AKIAIOSFODNN7EXAMPLE` (4-letter `AKIA` prefix + 16 uppercase-alnum = 20 chars total) and secret-access-key example `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` (40 chars, mixed-case + digit + `/` — base64-alphabet-shaped, no `+`/`=` in this particular example but the charset per AWS tooling is the standard base64 alphabet `[A-Za-z0-9+/]`).

No code mutations in this turn — I am the Architect; my role is the design decision record, not implementation. Developer applies the diff below.

## 1. Commit + branch identity

No code mutations in this turn. Architect does not write code.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | (none — design only) |

## 3. Acceptance criteria verification (design-readiness check, not implementation proof)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `redact.go` gets an AWS access-key-id pattern (`AKIA[0-9A-Z]{16}`) | DESIGNED | See §4 regex spec below; verified against AWS's own documented example shape (`AKIAIOSFODNN7EXAMPLE`, 20 chars) fetched this turn from docs.aws.amazon.com |
| AC2 | `redact.go` gets a label-anchored 40-char AWS secret-key pattern | DESIGNED | See §4; anchoring is mandatory per PM's regression finding — verified the literal string `"commit 0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"` does NOT contain any of the label tokens (`secret`, `access`, `key` as a labeled prefix), so the anchored pattern cannot match it. Confirmed by inspection of the regex and the test string side-by-side (not executed — I have no code-execution tool; Developer must run `go test` to confirm, see §6 gate) |
| AC3 | `redact_test.go` gets new positive + negative table cases | DESIGNED | Table spec in §5, follows the existing `TestRedactRemovesOtherShapes` / `TestRedactLeavesOtherTextAlone` structure verified in §0 |
| AC4 | `events.go`, `provisioner.go`, key-rotation/revocation untouched | DESIGNED (scope fence) | Diff surface limited to exactly 2 files per §2 target list below — Developer's diff must show zero hunks elsewhere |
| AC5 | Gate is `make check` + `go test ./pkg/api/logger/...` only | DESIGNED (constraint carried forward) | Sourced directly from `CLAUDE.md` read this turn (§0); no broader command may run |

This turn is a design handoff — ACs will be re-verified with real command output by Developer/QA against the actual diff. I mark them DESIGNED rather than PASS because I did not execute anything; PASS belongs to whoever runs the test.

## 4. Regex design (the two new patterns)

Target file: `pkg/api/logger/redact.go`, inside the existing `var (...)` block, same style as `privateKeyID`.

```go
// An AWS access key id. The AKIA prefix is a fixed, real AWS convention
// (distinct from the ASIA/temporary-credential prefix), so this is safe to
// match unscoped — see docs.aws.amazon.com AKIAIOSFODNN7EXAMPLE example.
awsAccessKeyID = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
// An AWS secret access key, anchored to a recognizable label the same way
// privateKeyID is anchored to "private_key_id" — an unscoped 40-char
// base64-alphabet match would catch ordinary hex strings such as commit SHAs
// (see redact_test.go TestRedactLeavesOtherTextAlone) and is rejected by design.
awsSecretAccessKey = regexp.MustCompile(`(?i)((?:aws_)?secret[_ ]?access[_ ]?key(?:\\*")?\s*[:=]\s*(?:\\*")?)[A-Za-z0-9+/]{40}`)
```

And in `Redact()`, inserted alongside the existing chain (order doesn't matter relative to `base64Run` since a 40-char match never satisfies `base64Run`'s `{100,}` minimum — no interaction):

```go
s = awsAccessKeyID.ReplaceAllString(s, redacted)
s = awsSecretAccessKey.ReplaceAllString(s, "${1}"+redacted)
```

Design notes / open judgment calls flagged for 1-line review rather than back-and-forth:

- **Label vocabulary covered:** `secret_access_key`, `SecretAccessKey`, `aws_secret_access_key`, `Secret Access Key`, `secret-access-key` — the `[_ ]?` alternation plus `(?i)` covers snake_case, PascalCase-as-typed, space-separated, and the optional `aws_` prefix. Does NOT cover a bare `kebab-case` separator (`secret-access-key`); if Developer finds that shape appears in practice (e.g. a Pulumi diff renders it that way), extend `[_ ]` to `[_ -]` — flagging this as the one spot that may need a 1-line tweak during implementation, not a design gap I'm hiding.
- **Quote-escaping handling:** mirrors `privateKeyID`'s `(?:\\*")?` exactly, so JSON-escaped-N-times label values (the same shape `TestRedactRemovesOtherShapes`'s `"JSON escaped three times"` case exercises for private keys) are handled identically.
- **Why not reuse `base64Run`'s decode-and-inspect approach for the secret key** (i.e. decode and check for an AWS-specific marker the way `holdsPrivateKey` checks for `"PRIVATE KEY"`)? Because a raw AWS secret key has no embedded self-describing marker when base64-decoded — unlike a PEM block, its 40 chars are opaque, so there's nothing to grep for after decoding. Label-anchoring is the only viable signal, which is exactly the trade-off PM already identified.
- **Order of insertion in the `var` block:** I suggest directly below `base64Run` so the three "specific identifier" patterns (`privateKeyID`, `awsAccessKeyID`, `awsSecretAccessKey`) aren't literally adjacent-but-logically-separate from the PEM-block patterns — purely cosmetic, Developer may place them wherever reads cleanest.

## 5. Test design (new table cases for `redact_test.go`)

Following the existing table-driven style (`TestRedactRemovesOtherShapes` map-of-name-to-text, `TestRedactLeavesOtherTextAlone` slice-of-strings):

**Positive cases** (new entries in a `TestRedactRemovesAWSCredentials`-style test, or added to `TestRedactRemovesOtherShapes`'s map):
1. Bare `AKIAIOSFODNN7EXAMPLE` in isolation → redacted.
2. `AKIAIOSFODNN7EXAMPLE` inside a Pulumi-diff-style line, e.g. `"      - access_key_id              : \"AKIAIOSFODNN7EXAMPLE\"\n"` → redacted, surrounding diff markers preserved.
3. `secret_access_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"` → value redacted, label text `secret_access_key:` preserved.
4. `AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` (env-style, no quotes) → value redacted.
5. A Pulumi diff line: `"      + SecretAccessKey              : \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n"` → value redacted, diff markers + label preserved.

**Negative cases** (added to `TestRedactLeavesOtherTextAlone`'s slice):
6. The existing `"commit 0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"` case — already present, now doubles as the AWS-pattern regression guard; Developer must confirm it still passes unchanged after the new patterns land.
7. A new case: a 20-char string that looks access-key-ish but lacks the `AKIA` prefix, e.g. `"user id: BBBB1234567890123456"` → must NOT be touched (proves the pattern isn't accidentally unscoped).
8. A new case: an unlabeled 40-char base64-ish string with no surrounding label context, e.g. `"token: aGVsbG8gd29ybGQgdGhpcyBpcyBub3RhU2VjcmV0S2V5"` (not a real secret, just base64 filler) → must NOT be touched, proving the label anchor is actually load-bearing and not accidentally satisfied by proximity alone.

Developer: place the AWS-specific cases in a new `TestRedactRemovesAWSCredentials` function rather than overloading `TestRedactRemovesOtherShapes` (which is scoped to the private-key-adjacent shapes) — keeps the diff reviewable and each test's name honest about what it covers. The negative cases, though, belong in the existing `TestRedactLeavesOtherTextAlone` since the existing commit-SHA regression guard already lives there.

## 6. Scope fence (restated from PM, unchanged — I reviewed and agree)

| Item | Status |
|---|---|
| `pkg/api/logger/redact.go` | ONLY file with production-code changes |
| `pkg/api/logger/redact_test.go` | ONLY file with test changes |
| `events.go`, `provisioner.go` | **MUST NOT be touched** — already sit behind the redaction chokepoint per the brief's own citation (`pkg/api/logger/logger.go:69` wraps `Info()` in `Redact()`), confirmed structurally consistent with the `redact.go` I read this turn (the function signature `func Redact(s string) string` is exactly what a logger wrapper would call) |
| Key rotation/revocation (S1/S2) | **MUST NOT be touched** — operator-owned |
| Unscoped 40-char secret pattern | **REJECTED design** — breaks `TestRedactLeavesOtherTextAlone`'s commit-SHA case; do not resurrect |
| Broader AWS shapes (STS session tokens, temporary `ASIA...` creds) | **OUT OF SCOPE for this slice** — not named in brief; record as a follow-up if operator wants it later |

## 7. Tests run

None — Architect turn, no code touched, no execution tooling available to me. Developer's gate (unchanged from PM's spec, carried forward from `CLAUDE.md` read in §0):

```sh
make check                          # go vet ./..., ~2.3GB peak — proven safe
go test ./pkg/api/logger/...        # scoped test run — do not broaden to ./...
```

Do not run `make build`, `make build-all`, bare `go build ./...`, or bare `go test ./...` — these link `cmd/sc` and have killed the Developer 4 times already on this exact slice via guest OOM.

**Empty-PR-OK:** yes — this is a design-only Architect turn with no source changes; all grounding (`redact.go`, `redact_test.go`, `preview_redact_test.go`, `CLAUDE.md`, AWS IAM docs) was done via read-only tool calls this turn, not mutation.

**Verdict:** signoff