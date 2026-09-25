Now I have a thorough picture of the codebase. Let me produce the PM handoff.

---

# PM Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Workflow run:** `08762da2-22ac-4162-abc6-60b46e7a89c5`
**Date:** 2026-06-12
**Author:** Max Warner (pm)
**Status:** Scoped ✅ — ready for developer

---

## 1. Problem Statement (one line)

`EncryptLargeString` chunks by **rune count** instead of **byte count**, causing `rsa.EncryptOAEP` to receive payloads that exceed the OAEP-SHA256 byte limit whenever a chunk contains multi-byte UTF-8 characters.

---

## 2. Scope Decision Table

| # | Item | Label | Rationale |
|---|------|-------|-----------|
| 1 | Fix `EncryptLargeString`: chunk by bytes using `maxPlain = rsaKey.Size() - 2*sha256.Size() - 2` | **MUST** | Direct root cause; one-function change |
| 2 | Remove the `lo.ChunkString` (rune-based) call; replace with byte-slice loop | **MUST** | Required to implement #1 |
| 3 | Add regression test: 128-rune UTF-8 window > 190 bytes fails under old code, passes under new | **MUST** | Prevents silent re-regression; fast to write |
| 4 | Add guard: return explicit error if `maxPlain <= 0` (RSA key too small) | **MUST** | Defensive; 2 lines |
| 5 | Note `EncryptWithPublicRSAKey` hash inconsistency (SHA-512 vs SHA-256) in a `TODO` comment | **MUST** | Documents latent risk; zero cost |
| 6 | Standardise hash across `EncryptWithPublicRSAKey` / `EncryptLargeString` | **NICE-TO-HAVE** | Breaks existing ciphertext round-trip; separate PR |
| 7 | Full hybrid-encryption rewrite (AES-GCM / ChaCha20 for RSA path) | **NICE-TO-HAVE** | Longer-term; separate slice |
| 8 | Migrate `DecryptLargeString` to byte-native join | **NICE-TO-HAVE** | Current `strings.Join` of `string(chunk)` works; no functional regression |

---

## 3. Exact Change Required

**File:** `pkg/api/secrets/ciphers/encryption.go`
**Function:** `EncryptLargeString` — RSA branch only

### Before (lines ~147–157)

```go
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    chunks := lo.ChunkString(s, rsaKey.Size()/2)          // rune-count chunks — BUG
    res = make([]string, len(chunks))
    for idx, chunk := range chunks {
        encryptedData, err := rsa.EncryptOAEP(
            sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
        ...
        res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
    }
}
```

### After

```go
if rsaKey, ok := key.(*rsa.PublicKey); ok {
    // RSA-OAEP max plaintext = k - 2·hLen - 2 (SHA-256 → hLen=32).
    // Chunk by BYTES, not runes, to handle multi-byte UTF-8 safely.
    maxPlain := rsaKey.Size() - 2*sha256.Size() - 2 // 190 for 2048-bit
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
            return nil, errors.Wrap(err, "failed to encrypt secret")
        }
        res = append(res, base64.StdEncoding.EncodeToString(enc))
    }
}
```

Also add to `EncryptWithPublicRSAKey` (near line 86):

```go
// TODO: this uses SHA-512 (max plaintext 126 B for 2048-bit key) while
// EncryptLargeString uses SHA-256 (max 190 B). Standardise in a future PR;
// do NOT route payloads > 126 B through this function.
```

And **remove** the now-unused `"github.com/samber/lo"` import if it is no longer referenced elsewhere in the file. (Verify — `lo` may still be used elsewhere; if not, drop it to keep the build clean.)

---

## 4. Test Requirements

**File to modify:** `pkg/api/secrets/ciphers/encryption_test.go` (or add to `encryption_extra_test.go`)

| Test case | What it proves |
|-----------|----------------|
| `EncryptLargeString` with 120× `─` (U+2500, 3 bytes each = 360 bytes in 120 runes) | Old code would fail; new code must pass |
| `EncryptLargeString` with mixed ASCII + multi-byte (e.g. `→`, `—`, Cyrillic, emoji) totalling > 190 bytes in a single window | Confirms general UTF-8 safety |
| Round-trip: encrypt with new code, decrypt with `DecryptLargeString`, result == original string | Backward-compatible decryption |
| Input of exactly 190 bytes (all ASCII) — single chunk | Boundary: 190 B ≤ maxPlain → exactly 1 chunk |
| Input of 191 bytes — split into 2 chunks | Boundary: forces the loop to produce 2 chunks |

---

## 5. Backward Compatibility

| Scenario | Impact |
|----------|--------|
| Existing secrets encrypted with old (rune-based) chunks | **No change** — `DecryptLargeString` is chunk-list–driven; chunk sizes are independent |
| Re-encrypting (`sc secrets hide`) after the fix | Produces new byte-based chunks; decrypts correctly |
| `DecryptLargeString` code | **No changes required** |

---

## 6. Acceptance Criteria

1. `go test ./pkg/api/secrets/ciphers/...` passes with all new test cases green.
2. `printf '# %s\n' "$(python3 -c 'print("─"*120)')" > /tmp/secret.txt && sc secrets add /tmp/secret.txt` succeeds (smoke test).
3. No `lo.ChunkString` call remains in `EncryptLargeString`.
4. `maxPlain` is derived from `rsaKey.Size() - 2*sha256.Size() - 2` (not a magic constant).
5. `EncryptWithPublicRSAKey` carries the `TODO` hash-inconsistency comment.

---

## 7. Out of Scope (this slice)

- Hybrid encryption for RSA path (longer-term; separate slice)
- `EncryptWithPublicRSAKey` hash unification (separate PR; breaking change)
- Any changes to CLI layer (`sc secrets add/hide`)
- Any changes to decryption path

---

## 8. Handoff JSON

```json
{
  "run_id": "08762da2-22ac-4162-abc6-60b46e7a89c5",
  "from": "pm",
  "to": "developer",
  "date": "2026-06-12",
  "artifact": "pm-handoff",
  "bug": "sc secrets add/hide fails on UTF-8 secret files — crypto/rsa message too long",
  "affected_file": "pkg/api/secrets/ciphers/encryption.go",
  "affected_function": "EncryptLargeString",
  "fix_summary": "Replace lo.ChunkString (rune-based) with byte-slice loop using maxPlain = rsaKey.Size() - 2*sha256.Size() - 2",
  "test_file": "pkg/api/secrets/ciphers/encryption_test.go",
  "must_items": [1, 2, 3, 4, 5],
  "nice_to_have_items": [6, 7, 8],
  "backward_compatible": true,
  "slice_id": null
}
```

---

## 9. Next Step

**Developer:** implement the byte-based chunking change in `pkg/api/secrets/ciphers/encryption.go` and add the regression tests described in §4. This is a ~30-line surgical change. No API surface, no schema, no config changes. Ship it.