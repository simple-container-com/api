PR is live at https://github.com/simple-container-com/api/pull/321.

---

# Developer Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Workflow run:** `08762da2-22ac-4162-abc6-60b46e7a89c5`
**Date:** 2026-06-12
**Author:** David Black (developer)
**Status:** Implementation complete ✅ — tests green, committed and pushed, PR open

---

## What was done

### 1. Root cause fix — `pkg/api/secrets/ciphers/encryption.go`

Replaced the `lo.ChunkString` (rune-based) call in `EncryptLargeString` RSA branch with a byte-slice loop using the exact OAEP formula:

```go
maxPlain := rsaKey.Size() - 2*sha256.Size - 2  // 190 for 2048-bit key
```

**Note:** `sha256.Size` is an untyped integer constant (= 32), not a function — `sha256.Size()` would not compile.

The `lo` import was deliberately retained — `lo.Map` is still used in `DecryptLargeString`.

### 2. TODO comment — `EncryptWithPublicRSAKey`

Added the comment documenting the SHA-512 vs SHA-256 inconsistency, as scoped by PM/architect.

### 3. Regression tests — `pkg/api/secrets/ciphers/encryption_test.go`

Added `TestEncryptLargeStringUTF8` with 6 sub-tests:

| Sub-test | Input | Validates |
|---|---|---|
| box-drawing flood | 120 × U+2500 (360 bytes, 120 runes) | The original bug case |
| mixed multi-byte | `→—яя🔑` × 20 (~280 bytes) | General multi-byte UTF-8 |
| exactly 190 ASCII bytes | `strings.Repeat("a", 190)` | Boundary: 1 chunk |
| 191 ASCII bytes | `strings.Repeat("a", 191)` | Boundary: 2 chunks |
| empty string | `""` | Zero-chunk edge case |
| 4096-bit key | 300 × U+2500 (900 bytes) | `maxPlain = 446`; 3 chunks |

All 6 pass. All pre-existing tests pass.

---

## Commit and PR

- **Commit:** `75f5eddaed7dcb4772efe2a3238f64c0afa66bb7`
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/08762da2-22ac-4162-abc6-60b46e7a89c5`
- **PR:** https://github.com/simple-container-com/api/pull/321
- **Workflow run:** https://app.simple-forge.com/agent-workflow-runs/08762da2-22ac-4162-abc6-60b46e7a89c5

`PULL_REQUEST: https://github.com/simple-container-com/api/pull/321`

---

## One implementation note

The PM/architect spec used `sha256.Size()` (function call syntax). In Go's `crypto/sha256` package, `Size` is a typed constant (`= 32`), not a method. The correct expression is `2*sha256.Size` without parentheses. This was caught immediately by the compiler on the first test run and corrected.

---

## Out of scope (not done, as agreed)

- `DecryptLargeString` bytes-join cleanup (`bytes.Join` vs current `strings.Join(lo.Map(...))`)
- `EncryptWithPublicRSAKey` hash unification (SHA-512 → SHA-256) — breaking change
- Hybrid encryption for RSA path — separate slice