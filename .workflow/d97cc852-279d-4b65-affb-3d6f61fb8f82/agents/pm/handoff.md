I have everything I need. Let me now produce the PM handoff document.

---

# PM Handoff — Run `d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Date:** 2026-06-12
**Workflow repo:** `simple-container-com/api` · branch `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Character:** Max Warner (PM)

---

## 1 — Bug Statement (one line)

`EncryptLargeString` chunks the plaintext by **rune count** against a limit that is in **bytes**, causing `rsa.EncryptOAEP` to fail with "message too long" on any secret file containing multi-byte UTF-8 characters.

---

## 2 — Scope Decision

| # | Item | Label | Rationale |
|---|------|-------|-----------|
| 1 | Fix `EncryptLargeString`: chunk by bytes, use correct OAEP-SHA256 max (`k − 2·hLen − 2 = 190` for 2048-bit) | **MUST** | Direct root cause; one function, ~10 lines |
| 2 | Drop `lo.ChunkString` dependency from the RSA path | **MUST** | Required by fix; rune-based chunking is the bug |
| 3 | Add failing regression test: RSA encrypt/decrypt of 128-rune UTF-8 string (box-drawing `─` × 120) | **MUST** | Proves the bug is fixed; must be committed with the fix |
| 4 | Add note / `//nolint` on `EncryptWithPublicRSAKey`'s SHA-512 inconsistency | **NICE-TO-HAVE** | Latent inconsistency; no known crash path today; document it, don't fix it in this PR |
| 5 | Migrate RSA path to hybrid encryption (AES-GCM / ChaCha20) | **CUT** | Correct long-term direction but out of scope for a bug fix; schedule separately |
| 6 | Force-re-encrypt all existing secrets in stores | **CUT** | Operational concern for users; backward compat is guaranteed without it |

---

## 3 — Exact Change Required

### File: `pkg/api/secrets/ciphers/encryption.go`

**Replace** the RSA branch inside `EncryptLargeString` (currently ≈ lines 147–159):

```go
// BEFORE (buggy)
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    chunks := lo.ChunkString(s, rsaKey.Size()/2)          // rune-count chunk — WRONG
    res = make([]string, len(chunks))
    for idx, chunk := range chunks {
        encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
        ...
        res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
    }
}
```

```go
// AFTER (correct)
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    // RSA-OAEP max plaintext = k − 2·hLen − 2.
    // SHA-256 → hLen=32; 2048-bit key → k=256; max = 190 bytes.
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

**Import cleanup:** remove `"github.com/samber/lo"` if it is no longer used elsewhere in the file (check: `lo.Map` is still used in `DecryptLargeString` — keep the import, just confirm `lo.ChunkString` is the only removed call).

> Note on `sha256.Size`: the `crypto/sha256` package exports the constant `sha256.Size = 32`. Use that rather than a magic number.

---

## 4 — Test Required (regression gate)

### File: `pkg/api/secrets/ciphers/encryption_test.go` (or a new `encryption_utf8_test.go`)

Add the following test cases inside the existing `TestRSAEncryptionDecryption` suite **or** as a new top-level function:

```go
t.Run("RSA encrypt/decrypt UTF-8 multi-byte string (box-drawing)", func(t *testing.T) {
    // U+2500 BOX DRAWINGS LIGHT HORIZONTAL = 3 bytes in UTF-8.
    // 120 runes × 3 bytes = 360 bytes > old 128-rune chunk limit (was failing).
    utf8Heavy := strings.Repeat("─", 120)  // 360 bytes

    encryptedChunks, err := EncryptLargeString(pubKey, utf8Heavy)
    Expect(err).To(BeNil())
    Expect(encryptedChunks).NotTo(BeEmpty())

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(utf8Heavy))
})

t.Run("RSA encrypt/decrypt emoji-dense string", func(t *testing.T) {
    // U+1F600 GRINNING FACE = 4 bytes. 100 runes = 400 bytes.
    emojiHeavy := strings.Repeat("😀", 100)  // 400 bytes

    encryptedChunks, err := EncryptLargeString(pubKey, emojiHeavy)
    Expect(err).To(BeNil())

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(emojiHeavy))
})

t.Run("RSA chunk boundaries do not corrupt multi-byte sequences", func(t *testing.T) {
    // Fill exactly 2 chunks worth of 3-byte runes; boundary falls mid-sequence.
    // 190 bytes per chunk × 2 = 380 bytes needed; 127 × 3 = 381 bytes.
    crossBoundary := strings.Repeat("─", 127)  // 381 bytes → spans 2 chunks

    encryptedChunks, err := EncryptLargeString(pubKey, crossBoundary)
    Expect(err).To(BeNil())
    Expect(len(encryptedChunks)).To(Equal(2))  // must produce exactly 2 chunks

    decrypted, err := DecryptLargeString(privKey, encryptedChunks)
    Expect(err).To(BeNil())
    Expect(string(decrypted)).To(Equal(crossBoundary))
})
```

---

## 5 — Acceptance Criteria (definition of done)

| # | Criterion |
|---|-----------|
| AC-1 | `go test ./pkg/api/secrets/ciphers/...` passes, including all new UTF-8 cases |
| AC-2 | The three new test cases each fail on the old code (verified by reverting the fix temporarily or by inspection of the math) |
| AC-3 | Existing `TestRSAEncryptionDecryption / RSA encrypt/decrypt large string` still passes (backward compat) |
| AC-4 | `lo.ChunkString` is removed from the RSA encryption path |
| AC-5 | No new external dependencies introduced |

---

## 6 — Critical-Path Sequence

```
1. Engineer applies fix to EncryptLargeString (pkg/api/secrets/ciphers/encryption.go)
2. Engineer adds failing-then-passing regression tests
3. go test ./pkg/api/secrets/ciphers/... — must be green
4. PR opened; AC-1 through AC-5 verified in CI
5. Merge → done
```

No design doc, Swagger regen, or roadmap update required (this is a bug fix in an internal cipher utility, not a public API or roadmap feature).

---

## 7 — Secondary Note (do NOT block merge on this)

`EncryptWithPublicRSAKey` uses SHA-512 (hLen=64 → max 126 B for 2048-bit) while `EncryptLargeString` uses SHA-256 (hLen=32 → max 190 B). These are inconsistent. Add a `// TODO:` comment noting this. Do **not** change `EncryptWithPublicRSAKey`'s hash in this PR — that would break existing encrypted payloads that use it.

---

## 8 — Handoff JSON

```json
{
  "run_id": "d97cc852-279d-4b65-affb-3d6f61fb8f82",
  "from_character": "pm",
  "to_character": "engineer",
  "date": "2026-06-12",
  "bug": "EncryptLargeString RSA path chunks by rune count, overflows OAEP byte limit on UTF-8 input",
  "affected_file": "pkg/api/secrets/ciphers/encryption.go",
  "affected_function": "EncryptLargeString",
  "fix_summary": "Replace lo.ChunkString(s, rsaKey.Size()/2) rune-chunking with byte-slice chunking using maxPlain = rsaKey.Size() - 2*sha256.Size - 2",
  "test_file": "pkg/api/secrets/ciphers/encryption_test.go",
  "new_test_cases": [
    "RSA encrypt/decrypt UTF-8 multi-byte string (box-drawing)",
    "RSA encrypt/decrypt emoji-dense string",
    "RSA chunk boundaries do not corrupt multi-byte sequences"
  ],
  "must_items": [1, 2, 3],
  "cut_items": [5, 6],
  "acceptance_criteria": ["AC-1", "AC-2", "AC-3", "AC-4", "AC-5"],
  "no_new_deps": true,
  "backward_compatible": true
}
```