All CI checks are settled. 20/21 pass; only DCO fails — and that is a known infrastructure limitation of forge-agent commits (no `Signed-off-by` trailer). Every substantive check — build, tests, CodeQL, vulncheck, security scans, semgrep — is green. The code is correct.

Now I'll write the QA handoff document.

---

# QA Handoff — Run `d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Date:** 2026-06-12
**From:** Maria Currie (QA)
**PR:** https://github.com/simple-container-com/api/pull/319
**Commit:** `e120d5f3dc9d1b5a913deec3409b29fe97013a94`
**Workflow run:** https://app.simple-forge.com/agent-workflow-runs/d97cc852-279d-4b65-affb-3d6f61fb8f82

PULL_REQUEST: https://github.com/simple-container-com/api/pull/319

---

**Verdict:** signoff

---

## 1 — What I Verified

I read both committed files directly from the branch and independently verified every acceptance criterion. I also waited for all CI checks to complete before issuing this verdict.

---

## 2 — Code Review: `pkg/api/secrets/ciphers/encryption.go`

### Fix correctness — PASS

The `EncryptLargeString` RSA branch now uses:

```go
maxPlain := rsaKey.Size() - 2*sha256.Size - 2
```

Math checked: `256 − 2×32 − 2 = 190` bytes for a 2048-bit key. This is the exact RSA-OAEP-SHA256 maximum and matches the formula in the brief.

The byte-slice loop is correct:

```go
data := []byte(s)
res = make([]string, 0, (len(data)+maxPlain-1)/maxPlain)
for i := 0; i < len(data); i += maxPlain {
    end := i + maxPlain
    if end > len(data) { end = len(data) }
    enc, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, data[i:end], nil)
    ...
}
```

- Chunk size is bounded by `maxPlain = 190` → no chunk can exceed the OAEP limit. ✓
- Pre-allocated slice with exact capacity. ✓
- Key-too-small guard for degenerate cases. ✓
- `errors.Errorf` / `errors.Wrap` from `github.com/pkg/errors` (already imported). ✓

### Import hygiene — PASS

`lo.ChunkString` removed from encryption path; `lo` import retained because `lo.Map` is still used in `DecryptLargeString`. ✓ (`grep ChunkString` → 0 hits confirmed by reading the file.)

### `DecryptLargeString` unchanged — PASS

Decryption joins bytes via `strings.Join(lo.Map(decrChunks, ...))`. Because Go strings are byte sequences, this reassembles the original byte stream correctly even when a chunk boundary falls mid-rune. ✓

### TODO comment on `EncryptWithPublicRSAKey` — PASS

Comment is present and accurate:

> TODO: This uses SHA-512 (max plaintext 126 B for 2048-bit keys) while EncryptLargeString uses SHA-256 (max 190 B). Standardize these in a future PR; changing the hash here would break decryption of existing payloads.

No functional change to that function. ✓

### ed25519 path — PASS

Untouched. ✓

---

## 3 — Code Review: `pkg/api/secrets/ciphers/encryption_test.go`

Three new sub-tests added inside `TestRSAEncryptionDecryption`.

### Test 1: `RSA encrypt/decrypt UTF-8 multi-byte string (box-drawing)`
- Payload: `strings.Repeat("─", 120)` = 120 × 3 bytes = **360 bytes**
- Old code: all 120 runes in one chunk → 360 B > 190 B limit → would have failed. ✓
- New code: ceil(360/190) = 2 chunks, both ≤ 190 B → succeeds. ✓
- Round-trip `Expect(string(decrypted)).To(Equal(utf8Heavy))` — correct. ✓

### Test 2: `RSA encrypt/decrypt emoji-dense string`
- Payload: `strings.Repeat("😀", 100)` = 100 × 4 bytes = **400 bytes**
- Old code: one 400-byte chunk → OAEP fail. ✓
- New code: ceil(400/190) = 3 chunks. ✓

### Test 3: `RSA chunk boundaries do not corrupt multi-byte sequences`
- Payload: `strings.Repeat("─", 64)` = 64 × 3 bytes = **192 bytes**
- Expected chunks: ceil(192/190) = **2** — assertion `Expect(len(encryptedChunks)).To(Equal(2))` is mathematically correct. ✓
- Chunk 1: bytes[0:190] — ends at offset 1 within rune 64 (bytes: `...E2`), i.e., mid-rune. ✓
- Chunk 2: bytes[190:192] — completes rune 64 (`94 80`). ✓
- Round-trip reassembly confirms byte-transparent join works. ✓

**Note:** The developer correctly caught and fixed an error in the architect's spec. The architect specified 127 runes = 381 bytes and claimed 2 chunks; `ceil(381/190) = 3`, not 2. The committed test uses 64 runes = 192 bytes → exactly 2 chunks. This is the correct value and the assertion will not flake.

### Existing test `RSA encrypt/decrypt large string` — PASS
- Payload: `testData` (57 ASCII bytes) × 20 = 1,140 bytes → ceil(1140/190) = 6 chunks, all ≤ 190 B. Backward compat intact. ✓

---

## 4 — Acceptance Criteria Verdict

| # | Criterion | Result | Evidence |
|---|-----------|--------|---------|
| AC-1 | `go test ./pkg/api/secrets/ciphers/...` passes green | **PASS** | CI "Run tests": SUCCESS |
| AC-2 | New UTF-8 tests fail on old code | **PASS** | Math: 360 B / 400 B / 192 B all exceed old 128-rune=128-B-per-chunk limit |
| AC-3 | Existing `RSA encrypt/decrypt large string` still passes | **PASS** | CI "Run tests": SUCCESS; ASCII payload unaffected |
| AC-4 | `lo.ChunkString` removed from RSA encryption path | **PASS** | Read code: byte-slice loop used; no `ChunkString` call |
| AC-5 | No new external dependencies | **PASS** | `sha256.Size` is stdlib; CI build + mod tidy clean |

All 5 ACs met.

---

## 5 — CI Check Summary (final state)

| Check | Result |
|-------|--------|
| Build Setup (clean, tools, schemas, lint, fmt) | SUCCESS |
| Run tests | SUCCESS |
| Build sc for linux/amd64 | SUCCESS |
| Build sc for darwin/arm64 | SUCCESS |
| Build sc for darwin/amd64 | SUCCESS |
| Build cloud-helpers | SUCCESS |
| Build github-actions | SUCCESS |
| Build github-actions-staging | SUCCESS |
| Finalize build in branch | SUCCESS |
| CodeQL | SUCCESS |
| govulncheck (reachability-aware) | SUCCESS |
| Fuzz HMAC cache parse path | SUCCESS |
| Semgrep | SUCCESS |
| security / Secret Scan (TruffleHog) | SUCCESS |
| security / SBOM + Dependency Scan | SUCCESS |
| security / Security Status | SUCCESS |
| **Developer Certificate of Origin** | **FAILURE** |

The DCO failure is an infrastructure issue: forge-agent commits do not include a `Signed-off-by` trailer. This is not a code defect and is the same pattern as every other forge-agent commit in this branch. All substantive checks — build, tests, security, static analysis — are green.

---

## 6 — Findings

No blocking findings. One informational note for the record:

1. **(Info) Architect spec error, caught by developer.** The cross-boundary test spec stated 127 runes = 381 bytes → 2 chunks; `ceil(381/190) = 3`, not 2. The developer caught this and used 64 runes (192 bytes → 2 chunks) correctly. The test assertion is deterministic and will not flake.

2. **(Info) DCO failure is infrastructure-only.** All functional checks green. No action needed from the engineering team on the code itself.

---

## 7 — Sign-off Scope

This review covers:
- `pkg/api/secrets/ciphers/encryption.go` — fix and TODO comment
- `pkg/api/secrets/ciphers/encryption_test.go` — three new regression tests
- Full CI check suite (21 checks, 20 green, 1 infrastructure-only DCO failure)

The fix is minimal, correct, backward-compatible, and fully covered by regression tests. Ready for merge.