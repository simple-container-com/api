Both files committed by David Black in this run. Here is the handoff.

---

# Developer Handoff — Run `d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Date:** 2026-06-12
**From:** David Black (developer)
**Repo:** `simple-container-com/api`
**Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/d97cc852-279d-4b65-affb-3d6f61fb8f82`
**Commit:** `e120d5f3dc9d1b5a913deec3409b29fe97013a94`
**PR:** https://github.com/simple-container-com/api/pull/319
**Workflow run:** https://app.simple-forge.com/agent-workflow-runs/d97cc852-279d-4b65-affb-3d6f61fb8f82

PULL_REQUEST: https://github.com/simple-container-com/api/pull/319

---

## What was done

### Root cause

`EncryptLargeString` in `pkg/api/secrets/ciphers/encryption.go` used `lo.ChunkString(s, rsaKey.Size()/2)` to split plaintext. `lo.ChunkString` counts **runes**. For a 2048-bit key `rsaKey.Size() = 256`, so chunk size = 128 runes. A 128-rune string of 3-byte UTF-8 characters (e.g. box-drawing `─`) encodes to 384 bytes — far above the RSA-OAEP-SHA256 maximum of `256 − 64 − 2 = 190 bytes`. Result: `crypto/rsa: message too long for RSA key size`.

### Fix applied — `pkg/api/secrets/ciphers/encryption.go`

Replaced the entire RSA branch of `EncryptLargeString`:

```go
// BEFORE (buggy — rune-count chunking)
chunks := lo.ChunkString(s, rsaKey.Size()/2)
res = make([]string, len(chunks))
for idx, chunk := range chunks {
    encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
    ...
    res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
}

// AFTER (correct — byte-slice chunking with exact OAEP limit)
maxPlain := rsaKey.Size() - 2*sha256.Size - 2   // 190 bytes for 2048-bit key
if maxPlain <= 0 {
    return nil, errors.Errorf("RSA key too small (%d bits) for OAEP-SHA256", rsaKey.Size()*8)
}
data := []byte(s)
res = make([]string, 0, (len(data)+maxPlain-1)/maxPlain)
for i := 0; i < len(data); i += maxPlain {
    end := i + maxPlain
    if end > len(data) { end = len(data) }
    enc, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, data[i:end], nil)
    if err != nil {
        return nil, errors.Wrap(err, "failed to encrypt secret")
    }
    res = append(res, base64.StdEncoding.EncodeToString(enc))
}
```

Also added a `// TODO:` comment on `EncryptWithPublicRSAKey` documenting the SHA-512/SHA-256 inconsistency. No functional change to that function.

**`lo` import retained** — `lo.Map` is still used in `DecryptLargeString`.

**`DecryptLargeString` unchanged** — decryption is chunk-list driven and chunk-size agnostic; old (rune-chunked) ciphertext decrypts correctly.

### Tests added — `pkg/api/secrets/ciphers/encryption_test.go`

Three new sub-tests inside `TestRSAEncryptionDecryption`:

| Test name | Payload | Why |
|-----------|---------|-----|
| `RSA encrypt/decrypt UTF-8 multi-byte string (box-drawing)` | 120 × `─` = 360 bytes | Reproduces the original bug (was failing) |
| `RSA encrypt/decrypt emoji-dense string` | 100 × `😀` = 400 bytes | 4-byte code points, higher density |
| `RSA chunk boundaries do not corrupt multi-byte sequences` | 64 × `─` = 192 bytes → 2 chunks (190+2) | Byte boundary falls mid-rune; proves join is safe |

Note on chunk-count math: the architect's handoff specified 127 runes = 381 bytes → 2 chunks. That is wrong — `ceil(381/190) = 3`. The correct value is 64 runes = 192 bytes → `ceil(192/190) = 2`. Fixed before committing; test passes.

---

## Acceptance criteria — all met

| # | Criterion | Status |
|---|-----------|--------|
| AC-1 | `go test ./pkg/api/secrets/ciphers/...` passes green | ✅ `ok 2.977s` |
| AC-2 | New UTF-8 tests fail on old code | ✅ (360-byte chunk > 190-byte limit by math) |
| AC-3 | Existing `RSA encrypt/decrypt large string` still passes | ✅ |
| AC-4 | `lo.ChunkString` removed from encryption path | ✅ `grep ChunkString` → 0 hits |
| AC-5 | No new external dependencies | ✅ `go mod tidy` clean |

---

## What was NOT changed

- `DecryptLargeString` — byte-transparent; no change required
- `EncryptWithPublicRSAKey` / `DecryptWithPrivateRSAKey` — SHA-512 single-shot path; TODO comment added, no functional change
- ed25519 hybrid path — already correct; untouched
- Public API surface, CLI flags, or wire format