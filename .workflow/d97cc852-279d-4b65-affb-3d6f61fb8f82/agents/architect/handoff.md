Good — I have everything I need. I've read the actual source code and tests. No CLAUDE.md exists in this repo. Let me now produce the architectural review and handoff.

---

# Architect Handoff — Run `d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Date:** 2026-06-12
**From:** Dan Johnson (architect)
**To:** engineer
**Repo:** `simple-container-com/api` · branch `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/d97cc852-279d-4b65-affb-3d6f61fb8f82`

---

## 1 — Source Review: What I Actually Read

I fetched and read both files directly from the branch. Key observations:

### `pkg/api/secrets/ciphers/encryption.go` (current buggy state)

```go
func EncryptLargeString(key crypto.PublicKey, s string) ([]string, error) {
    var res []string
    if rsaKey, ok := key.(*rsa.PublicKey); ok {
        chunks := lo.ChunkString(s, rsaKey.Size()/2)   // BUG: rune-count, not bytes
        res = make([]string, len(chunks))
        for idx, chunk := range chunks {
            encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
            ...
        }
    }
```

**Confirmed** — `lo.ChunkString` operates on `[]rune`. For a 2048-bit key: `rsaKey.Size() = 256`, so chunk size = 128 runes. A 128-rune chunk of 3-byte UTF-8 characters = 384 bytes — well above OAEP-SHA256's 190-byte limit. The bug is exactly as described.

**Also confirmed** — `lo.Map` **is** used in `DecryptLargeString` (line: `lo.Map(decrChunks, func(chunk []byte, _ int) string {...})`), so the `lo` import **must be retained** after the fix.

### `pkg/api/secrets/ciphers/encryption_test.go` (current test state)

Existing test `TestRSAEncryptionDecryption / RSA encrypt/decrypt large string` uses `strings.Repeat(testData, 20)` where `testData = "Hello, World! This is a test message for RSA encryption."` — entirely ASCII. This **will not** catch the UTF-8 bug. The regression tests PM specified are necessary and correct.

---

## 2 — Architectural Decision Record

### ADR-001: Chunk by bytes, not runes

**Decision:** Replace `lo.ChunkString(s, rsaKey.Size()/2)` with a manual byte-slice loop using `maxPlain = rsaKey.Size() - 2*sha256.Size - 2`.

**Why this and not alternatives:**

| Option | Verdict | Reason |
|--------|---------|--------|
| Fix chunk size to bytes with correct OAEP limit | **ADOPT** | Minimal change, correct, backward-compatible decryption |
| Switch to hybrid encryption (RSA wraps AES key) | **DEFER** | Correct long-term, but touches decryption path, wire format, and existing stored secrets. Out of scope for a bug fix. |
| Increase key size | **REJECT** | Does not fix the root bug; just widens the window of failure |
| Pin chunk size to 126 bytes to match EncryptWithPublicRSAKey's SHA-512 limit | **REJECT** | Inconsistency in hash algo is a separate problem; do not conflate |

**Key math (must be exactly this — do not vary):**
- `k = rsaKey.Size()` = 256 bytes for 2048-bit key
- `hLen = sha256.Size` = 32 (stdlib constant from `crypto/sha256`)
- `maxPlain = k - 2*hLen - 2` = 256 - 64 - 2 = **190 bytes**
- Old code: 128 runes (safe only for ASCII, fails for multi-byte UTF-8)

**Backward compatibility:** Decryption is chunk-list driven and chunk-size agnostic — `DecryptLargeString` iterates whatever slices it receives. Old ciphertext (rune-chunked) still decrypts correctly. ✓

### ADR-002: Keep `lo` import

`lo.Map` is still used in `DecryptLargeString`. Only `lo.ChunkString` is removed from the RSA encryption path. **Do not remove the import.**

### ADR-003: No changes to `DecryptLargeString`

The decryption path is correct as-is. It uses byte-transparent `strings.Join` semantics — splitting at arbitrary byte offsets during encryption is safe because the join reassembles the original byte sequence.

### ADR-004: TODO comment on SHA inconsistency, not a fix

`EncryptWithPublicRSAKey` uses SHA-512 (max 126 B for 2048-bit), `EncryptLargeString` uses SHA-256 (max 190 B). This inconsistency is latent — changing `EncryptWithPublicRSAKey`'s hash would break existing SHA-512-encrypted payloads. Add a `// TODO:` comment only.

---

## 3 — Precise Implementation Spec for Engineer

### File: `pkg/api/secrets/ciphers/encryption.go`

**Change only the RSA branch of `EncryptLargeString`.** Do not touch `DecryptLargeString`, `EncryptWithPublicRSAKey`, or any ed25519 functions.

```go
// REMOVE this entire RSA block:
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    chunks := lo.ChunkString(s, rsaKey.Size()/2)
    res = make([]string, len(chunks))
    for idx, chunk := range chunks {
        encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
        if err != nil {
            return nil, errors.Wrapf(err, "failed to encrypt secret")
        }
        res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
    }
}

// REPLACE WITH:
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    // RSA-OAEP with SHA-256: max plaintext = k − 2·hLen − 2 bytes.
    // For a 2048-bit key: 256 − 64 − 2 = 190 bytes.
    // Chunking by rune count (old behaviour) is unsafe for multi-byte UTF-8:
    // a 128-rune chunk can be up to 512 bytes, exceeding the 190-byte OAEP limit.
    maxPlain := rsaKey.Size() - 2*sha256.Size - 2
    if maxPlain <= 0 {
        return nil, errors.Errorf("RSA key too small (%d bits) for OAEP-SHA256", rsaKey.Size()*8)
    }
    data := []byte(s)
    res = make([]string, 0, (len(data)+maxPlain-1)/maxPlain)
    for i := 0; i < len(data); i += maxPlain {
        end := i + maxPlain
        if end > len(data) {
            end = len(data)
        }
        enc, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, data[i:end], nil)
        if err != nil {
            return nil, errors.Wrap(err, "failed to encrypt secret")
        }
        res = append(res, base64.StdEncoding.EncodeToString(enc))
    }
}
```

**Also add a TODO to `EncryptWithPublicRSAKey`:**

```go
// EncryptWithPublicRSAKey encrypts data with public key
// TODO: This uses SHA-512 (max plaintext 126 B for 2048-bit keys) while
// EncryptLargeString uses SHA-256 (max 190 B). Standardize these in a future PR;
// changing the hash here would break decryption of existing payloads.
func EncryptWithPublicRSAKey(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
```

**Import verification:** `lo` must remain (still used in `DecryptLargeString`). No new imports needed — `sha256.Size` is already available via the existing `"crypto/sha256"` import.

---

## 4 — Test Spec for Engineer

### File: `pkg/api/secrets/ciphers/encryption_test.go`

Add inside or alongside the `TestRSAEncryptionDecryption` function. The key pair (`privKey`, `pubKey`) is already generated at the top of that function — reuse it. The test must use `RegisterTestingT(t)` and Gomega matchers to match the existing style.

```go
t.Run("RSA encrypt/decrypt UTF-8 multi-byte string (box-drawing)", func(t *testing.T) {
    // U+2500 BOX DRAWINGS LIGHT HORIZONTAL = 3 bytes UTF-8.
    // 120 runes × 3 bytes = 360 bytes.
    // Old rune-based chunking (128 runes/chunk) produced a 360-byte chunk > 190 B → FAIL.
    utf8Heavy := strings.Repeat("─", 120)

    encryptedChunks, err := EncryptLargeString(pubKey, utf8Heavy)
    Expect(err).To(BeNil())
    Expect(encryptedChunks).NotTo(BeEmpty())

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(utf8Heavy))
})

t.Run("RSA encrypt/decrypt emoji-dense string", func(t *testing.T) {
    // U+1F600 GRINNING FACE = 4 bytes UTF-8. 100 runes = 400 bytes.
    emojiHeavy := strings.Repeat("😀", 100)

    encryptedChunks, err := EncryptLargeString(pubKey, emojiHeavy)
    Expect(err).To(BeNil())
    Expect(encryptedChunks).NotTo(BeEmpty())

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(emojiHeavy))
})

t.Run("RSA chunk boundaries do not corrupt multi-byte sequences", func(t *testing.T) {
    // 127 × 3 bytes = 381 bytes → forces exactly 2 chunks (190 + 191).
    // Byte boundary at 190 falls mid-rune (U+2500 is bytes E2 94 80).
    // Correct decryption proves byte-boundary splitting + join works.
    crossBoundary := strings.Repeat("─", 127)

    encryptedChunks, err := EncryptLargeString(pubKey, crossBoundary)
    Expect(err).To(BeNil())
    Expect(len(encryptedChunks)).To(Equal(2))

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(crossBoundary))
})
```

**Chunk-count math for the boundary test:**
- `crossBoundary` = 381 bytes
- chunk 1: bytes [0:190] = 190 bytes
- chunk 2: bytes [190:381] = 191 bytes
- → exactly 2 chunks ✓

---

## 5 — Acceptance Checklist

| # | Check | How to verify |
|---|-------|--------------|
| AC-1 | `go test ./pkg/api/secrets/ciphers/...` passes green | Run locally + CI |
| AC-2 | New UTF-8 tests would FAIL on old code | Confirm by math (128-rune chunk = 384 B > 190 B limit) or revert temporarily |
| AC-3 | Existing `RSA encrypt/decrypt large string` still passes | Included in `go test` above |
| AC-4 | `lo.ChunkString` removed from encryption path | `grep -n ChunkString` returns 0 hits |
| AC-5 | `lo` import retained | `DecryptLargeString` still uses `lo.Map` |
| AC-6 | No new imports or dependencies | `go mod tidy` produces no changes |
| AC-7 | TODO comment added to `EncryptWithPublicRSAKey` | Code review |

---

## 6 — Structural Concerns / Watch-outs

1. **`sha256.Size` not `sha256.Size()`** — It's a constant (`= 32`), not a function. Using `sha256.New().Size()` also works but is verbose. `sha256.Size` is cleaner and idiomatic.

2. **`errors.Wrap` vs `errors.Wrapf`** — The existing code uses `errors.Wrapf`. The fix uses `errors.Wrap` for the encrypt error (no format args needed) and `errors.Errorf` for the key-too-small guard. Both are from `"github.com/pkg/errors"` which is already imported. Keep consistent.

3. **Don't touch `DecryptLargeString`** — The `lo.Map` there is fine. The byte-join semantics are correct. No change needed.

4. **Test registration** — The existing `TestRSAEncryptionDecryption` calls `RegisterTestingT(t)` at the top. Sub-tests inherit that registration. No additional `RegisterTestingT` calls needed inside `t.Run` blocks.

5. **Chunk-count assertion** — The `Expect(len(encryptedChunks)).To(Equal(2))` assertion is deterministic: 381 bytes / 190 = 2 chunks (190 + 191). This will not flake.

---

## 7 — What This Fix Does NOT Change

- Wire format for **decryption** — existing secrets remain decryptable
- The `ed25519` path — hybrid encryption is already correct; do not touch
- `EncryptWithPublicRSAKey` / `DecryptWithPrivateRSAKey` — SHA-512, single-shot, documented with TODO only
- Any public API, CLI flags, or command signatures
- No new dependencies, no go.mod changes

---

## 8 — Summary for Engineer

**Two files to touch, one structural change each:**

1. **`pkg/api/secrets/ciphers/encryption.go`**
   - Replace RSA branch of `EncryptLargeString` (byte-slice loop, `maxPlain = rsaKey.Size() - 2*sha256.Size - 2`)
   - Add `// TODO:` comment to `EncryptWithPublicRSAKey` documenting SHA inconsistency
   - Verify `lo` import is retained (it is, via `DecryptLargeString`)

2. **`pkg/api/secrets/ciphers/encryption_test.go`**
   - Add three sub-tests inside/adjacent to `TestRSAEncryptionDecryption`:
     - box-drawing (360 B, must succeed)
     - emoji (400 B, must succeed)
     - cross-boundary (381 B, must produce exactly 2 chunks and round-trip correctly)

**Run `go test ./pkg/api/secrets/ciphers/...` before handing off — it must be green.**