Good — I have the full source of both files. Now let me produce the architectural review.

---

# Architect Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Workflow run:** `08762da2-22ac-4162-abc6-60b46e7a89c5`
**Date:** 2026-06-12
**Author:** Dan Johnson (architect)
**Status:** Architecture review complete ✅ — cleared for implementation

---

## 1. Code Reality Check

I read the actual source at `pkg/api/secrets/ciphers/encryption.go`. The PM's analysis is **correct in every detail**. Confirming live against the file:

```go
// CONFIRMED in source — the offending line:
chunks := lo.ChunkString(s, rsaKey.Size()/2)
```

- `rsaKey.Size()` for a 2048-bit key = 256.
- `rsaKey.Size()/2` = 128.
- `lo.ChunkString` operates on **runes** (it converts to `[]rune` internally).
- The byte-level OAEP-SHA256 limit = 256 − 2×32 − 2 = **190 bytes**.
- A 128-rune window of 3-byte UTF-8 characters = up to 384 bytes >> 190 → guaranteed failure.

The bug is exactly as described. No ambiguity.

---

## 2. Architectural Assessment

### 2.1 The Two-Compounding-Bugs Pattern

```
Input string (UTF-8)
        │
        ▼
lo.ChunkString(s, 128)  ← counts RUNES, not bytes
        │
        ▼
[]byte(chunk)           ← silent expansion: 1 rune → up to 4 bytes
        │
        ▼
rsa.EncryptOAEP(sha256, data)  ← hard byte limit = 190 bytes
        │
        ▼
PANIC: message too long for RSA key size  (if chunk > 190 bytes)
```

The two bugs **compound**: the chunk size is already a rough heuristic (`Size()/2` is not the OAEP formula), and the rune-vs-byte mismatch turns it into a live correctness bug for any non-ASCII input.

### 2.2 Why the Fix is Safe

The proposed fix chunks by **bytes** at `maxPlain = rsaKey.Size() - 2*sha256.Size() - 2`:

- This is the exact OAEP formula from RFC 8017 §7.1.1. No magic constant.
- Splitting a UTF-8 multi-byte sequence across chunk boundaries is **safe** because `DecryptLargeString` reconstructs the original byte sequence: it decrypts each chunk to `[]byte`, and the existing `strings.Join(lo.Map(...))` path reassembles them. A split multi-byte sequence in chunk `n` and `n+1` is perfectly valid once concatenated.
- `DecryptLargeString` is chunk-size agnostic — it decrypts whatever list it receives. Existing ciphertexts produced by the rune-based chunking will still decrypt correctly.

### 2.3 Import Cleanup — Critical Check

`lo.ChunkString` is **the only use** of `github.com/samber/lo` in the RSA branch of `EncryptLargeString`. However, `lo.Map` is **still used in `DecryptLargeString`**:

```go
return []byte(strings.Join(lo.Map(decrChunks, func(chunk []byte, _ int) string {
    return string(chunk)
}), "")), nil
```

**Therefore: do NOT remove the `lo` import.** The import stays. The PM's caveat ("verify — `lo` may still be used elsewhere") applies: it is used.

### 2.4 `EncryptWithPublicRSAKey` — Hash Inconsistency

Confirmed in source:
```go
func EncryptWithPublicRSAKey(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
    hash := sha512.New()   // SHA-512: max 256 - 2*64 - 2 = 126 bytes for 2048-bit key
```

vs.

```go
// EncryptLargeString RSA branch:
rsa.EncryptOAEP(sha256.New(), ...)  // SHA-256: max 190 bytes for 2048-bit key
```

This is a latent inconsistency — not the active bug, but a trap for anyone who calls `EncryptWithPublicRSAKey` with payloads between 127–190 bytes expecting them to work. The `TODO` comment is the right call for this slice.

### 2.5 `DecryptLargeString` — No Changes Needed (But Note the Smell)

```go
return []byte(strings.Join(lo.Map(decrChunks, func(chunk []byte, _ int) string {
    return string(chunk)
}), "")), nil
```

This does a `[]byte → string → join → []byte` round-trip which is technically fine for valid UTF-8 but is mildly wasteful and semantically confusing (it converts binary data through the `string` type). This is a NICE-TO-HAVE cleanup — `bytes.Join(decrChunks, nil)` is cleaner — but it is **not blocking** for this fix. Do not change it in this slice.

---

## 3. Decision Record

| # | Decision | Rationale |
|---|----------|-----------|
| DR-1 | Chunk by bytes using `rsaKey.Size() - 2*sha256.Size() - 2` | Correct OAEP formula; removes all ambiguity |
| DR-2 | Drop `lo.ChunkString` call only, keep `lo` import | `lo.Map` still used in `DecryptLargeString` |
| DR-3 | No change to `DecryptLargeString` | Backward compat; chunk-size agnostic; byte reconstruction works |
| DR-4 | Add `maxPlain <= 0` guard | Defensive: explicit error beats panic for tiny/bad keys |
| DR-5 | Add TODO comment to `EncryptWithPublicRSAKey` | Documents SHA-512 vs SHA-256 inconsistency without breaking anything |
| DR-6 | Do NOT change `DecryptLargeString`'s `bytes.Join` pattern | Out of scope; no risk; separate cleanup |
| DR-7 | Do NOT standardise hash algorithm in this slice | Breaking change to existing ciphertexts; separate PR |

---

## 4. Complete Revised `EncryptLargeString` RSA Branch

This is the exact implementation the developer should produce:

```go
func EncryptLargeString(key crypto.PublicKey, s string) ([]string, error) {
    var res []string
    if rsaKey, ok := key.(*rsa.PublicKey); ok {
        // RSA-OAEP with SHA-256: max plaintext per chunk = k - 2*hLen - 2
        // (RFC 8017 §7.1.1), where k = modulus bytes, hLen = sha256.Size() = 32.
        // For a 2048-bit key: 256 - 64 - 2 = 190 bytes.
        // Chunk by BYTES (not runes) so multi-byte UTF-8 cannot overflow the limit.
        maxPlain := rsaKey.Size() - 2*sha256.Size() - 2
        if maxPlain <= 0 {
            return nil, errors.Errorf(
                "RSA key too small (%d bits) for OAEP-SHA256", rsaKey.Size()*8)
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
                return nil, errors.Wrapf(err, "failed to encrypt secret")
            }
            res = append(res, base64.StdEncoding.EncodeToString(enc))
        }
    } else if ed25519Key, ok := key.(ed25519.PublicKey); ok {
        // ed25519 branch unchanged — uses hybrid encryption (no chunking needed)
        encryptedData, err := encryptWithEd25519(ed25519Key, []byte(s))
        if err != nil {
            return nil, errors.Wrapf(err, "failed to encrypt secret with ed25519")
        }
        res = []string{base64.StdEncoding.EncodeToString(encryptedData)}
    } else {
        return nil, errors.New("unsupported key type for encryption")
    }
    return res, nil
}
```

And the TODO to add to `EncryptWithPublicRSAKey`:

```go
// EncryptWithPublicRSAKey encrypts data with public key using RSA-OAEP + SHA-512.
// TODO: this uses SHA-512 (max plaintext 126 B for a 2048-bit key) while
// EncryptLargeString uses SHA-256 (max 190 B). Standardise in a future PR.
// Do NOT route payloads > 126 B through this function.
func EncryptWithPublicRSAKey(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
```

---

## 5. Test Matrix (for developer reference)

| Test | Input | Expected |
|------|-------|----------|
| UTF-8 box-drawing flood | `strings.Repeat("─", 120)` = 360 bytes in 120 runes | Encrypt succeeds; round-trip passes |
| Mixed multi-byte | `→` `—` `→` Cyrillic `я` emoji `🔑` × N, total > 190 bytes | Encrypt succeeds; round-trip passes |
| Exactly 190 ASCII bytes | `strings.Repeat("a", 190)` | 1 chunk; encrypt + decrypt = original |
| 191 ASCII bytes | `strings.Repeat("a", 191)` | 2 chunks; encrypt + decrypt = original |
| 4096-bit key round-trip | Large UTF-8 payload, 4096-bit key | `maxPlain` = 446; fewer chunks; correct |
| Empty string | `""` | 0 chunks; decrypt returns `""` |
| Existing (rune-chunked) ciphertexts | Old chunks in `DecryptLargeString` | Decrypt unchanged; not tested in unit test but backward-compat by design |

The test for the old code failing is not necessary to include (it would require temporarily reverting the fix). A comment referencing the bug is sufficient documentation.

---

## 6. Potential Risks / Things Developer Should Watch

1. **`lo` import must stay** — removing it causes a compile error in `DecryptLargeString`.
2. **`errors.Wrapf` not `errors.Wrap`** — existing code uses `Wrapf` for the ed25519 branch; match the style.
3. **`rand.Reader` argument** — `rsa.EncryptOAEP` takes `io.Reader` as second arg. The existing code correctly passes `rand.Reader`; keep it.
4. **Test file package** — existing tests are `package ciphers` (same package, not `_test`), so they can directly call unexported helpers. New tests must match this.
5. **4096-bit key behaviour** — `maxPlain` for a 4096-bit key = 512 − 64 − 2 = 446. Tests covering both 2048 and 4096-bit keys are welcome but not required for this slice.

---

## 7. Out of Scope (Architect Concurs)

- **Hybrid encryption for RSA path** — The ed25519 path already uses ChaCha20-Poly1305 hybrid correctly. Migrating RSA to the same pattern would be the right long-term architecture but requires a migration plan for existing secrets. Separate slice.
- **`EncryptWithPublicRSAKey` hash unification** — Changing SHA-512→SHA-256 there would break the decrypt side for any data currently encrypted with that function. Separate PR with a migration.
- **`DecryptLargeString` cleanup** — `bytes.Join(decrChunks, nil)` is cleaner than the current `strings.Join(lo.Map(...))` path but has zero user impact. Separate cleanup.

---

## 8. Handoff to Developer

**The architectural review is clean. This is a surgical, well-understood fix:**

1. **File:** `pkg/api/secrets/ciphers/encryption.go`
2. **Change:** Replace the `lo.ChunkString` block in `EncryptLargeString` RSA branch with the byte-slicing loop above (§4).
3. **Do not remove the `lo` import** — it is still used by `DecryptLargeString`.
4. **Add TODO comment** to `EncryptWithPublicRSAKey` (§4).
5. **Tests:** Add the cases in §5 to `encryption_test.go`. Use `package ciphers`, `RegisterTestingT(t)`, gomega matchers — match existing style.
6. **Run:** `go test ./pkg/api/secrets/ciphers/...` must pass green before handoff.

No schema changes. No API surface changes. No config changes. No CLI changes. Ship it.