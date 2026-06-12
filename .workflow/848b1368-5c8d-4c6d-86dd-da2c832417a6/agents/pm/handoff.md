The workflow branch has the same unpatched `encryption.go` as `main`. I have a complete picture. Now I produce my PM handoff document.

---

# PM Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Run:** `848b1368-5c8d-4c6d-86dd-da2c832417a6`
**Date:** 2026-06-12
**Author:** Max Warner (pm)
**Status:** Scoped — ready for engineering execution

---

## 1. Problem Statement (one line)

`EncryptLargeString` chunks by **rune count** but RSA-OAEP enforces a **byte limit**, so any secret file with multi-byte UTF-8 characters triggers `crypto/rsa: message too long for RSA key size`.

---

## 2. Confirmed Diagnosis

| # | Finding | Evidence |
|---|---------|----------|
| 1 | `lo.ChunkString(s, rsaKey.Size()/2)` splits by rune count (128 runes for a 2048-bit key) | Source: `encryption.go` ≈ line 147 |
| 2 | OAEP-SHA256 max plaintext for 2048-bit key = **190 bytes** (`256 − 64 − 2`) | Math; `Size()/2 = 128` happens to be safe for ASCII but not UTF-8 |
| 3 | A 128-rune chunk of box-drawing chars (`─`, U+2500, 3 bytes each) → **384 bytes** → hard failure | Reproducer in brief |
| 4 | Existing test suite has no UTF-8 / multi-byte test cases for `EncryptLargeString` | Confirmed by reading `encryption_test.go` + `encryption_extra_test.go` |
| 5 | `DecryptLargeString` is unaffected — it is chunk-count-driven, not chunk-size-driven | Source: same file |
| 6 | Secondary latent issue: `EncryptWithPublicRSAKey` uses SHA-512 (limit 126 B) vs `EncryptLargeString` SHA-256 (limit 190 B) — hash inconsistency noted but callers only pass short payloads today | Out of scope for this fix |

---

## 3. Scope Decision Table

| Item | Priority | Rationale |
|------|----------|-----------|
| Fix `EncryptLargeString`: chunk by **bytes**, size = `rsaKey.Size() − 2*sha256.Size() − 2` (190 for 2048-bit) | **MUST** | Direct bug fix; unblocks affected users |
| Remove `lo.ChunkString` / `lo` import from RSA path (replace with byte-slice loop) | **MUST** | Required by the byte-chunking approach |
| Add regression tests: UTF-8 heavy (box-drawing, emoji, CJK, Cyrillic) round-trip via `EncryptLargeString` + `DecryptLargeString` | **MUST** | Prevents recurrence; no test existed |
| Guard: return error if key is too small for OAEP-SHA256 (`maxPlain <= 0`) | **MUST** | Defensive; already in proposed fix |
| Fix hash inconsistency in `EncryptWithPublicRSAKey` (SHA-512 vs SHA-256) | **NICE-TO-HAVE** | Latent; no current caller is affected at scale; do not block this fix on it |
| Migrate RSA path to full hybrid encryption (AES-GCM / ChaCha20-Poly1305) | **CUT** | Correct long-term direction but out of scope for a bug fix; scope creep risk |
| Update `DecryptLargeString` logic | **CUT** | No change needed; backward-compatible by design |

---

## 4. Exact Code Change Required

**File:** `pkg/api/secrets/ciphers/encryption.go`
**Function:** `EncryptLargeString` — RSA branch only

**Replace:**
```go
chunks := lo.ChunkString(s, rsaKey.Size()/2)
res = make([]string, len(chunks))
for idx, chunk := range chunks {
    encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
    if err != nil {
        return nil, errors.Wrapf(err, "failed to encrypt secret")
    }
    res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
}
```

**With:**
```go
// RSA-OAEP max plaintext = k − 2·hLen − 2.  For SHA-256 (hLen=32) and a
// 2048-bit key (k=256): 256 − 64 − 2 = 190 bytes.
maxPlain := rsaKey.Size() - 2*sha256.Size - 2
if maxPlain <= 0 {
    return nil, errors.Errorf("RSA key too small (%d bits) for OAEP-SHA256",
        rsaKey.Size()*8)
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
```

> **Note:** `sha256.Size` is the constant `32` exported by `crypto/sha256`. `lo` import can be removed from the RSA path (the ed25519 branch no longer uses `lo` either once this change lands; verify and drop the import if unused).

---

## 5. Required Tests (add to `encryption_test.go` or `encryption_extra_test.go`)

| Test name | Input | Assertion |
|-----------|-------|-----------|
| `TestEncryptLargeString_UTF8_BoxDrawing` | 256× `─` (U+2500, 3 bytes each = 768 bytes total) | Encrypt + Decrypt round-trips correctly, no error |
| `TestEncryptLargeString_UTF8_Emoji` | 100× `🔑` (U+1F511, 4 bytes each = 400 bytes) | Round-trip correct |
| `TestEncryptLargeString_UTF8_Mixed` | Real-world sample: section header with `─`, `→`, `↔`, `—`, Cyrillic | Round-trip correct |
| `TestEncryptLargeString_UTF8_AtBoundary` | Exactly 190 bytes of 3-byte chars (63 full chars + 1 byte) — chunk boundary falls mid-rune | Round-trip correct (byte split is safe) |
| `TestEncryptLargeString_SmallKey_TooSmall` | Generate a 512-bit key (if the guard triggers) | Returns error containing "RSA key too small" |

---

## 6. Acceptance Criteria

1. `go test ./pkg/api/secrets/ciphers/...` passes — all existing tests green, all new UTF-8 tests green.
2. The exact reproducer from the brief encrypts without error:
   ```sh
   printf '# %s\n' "$(python3 -c 'print("─"*120)')" > /tmp/secret.txt
   sc secrets add /tmp/secret.txt   # must succeed
   ```
3. Existing secrets encrypted with the old rune-based chunking still decrypt (backward-compat — verified by the unmodified `DecryptLargeString`).
4. No `lo.ChunkString` call remains in the RSA path.

---

## 7. What I Am NOT Asking For

- No hybrid-encryption refactor.
- No `EncryptWithPublicRSAKey` hash change.
- No changelog entry beyond a single line in whatever changelog convention the repo uses.
- No swagger regen (no API surface change).

---

## 8. Handoff JSON

```json
{
  "run_id": "848b1368-5c8d-4c6d-86dd-da2c832417a6",
  "character": "pm",
  "date": "2026-06-12",
  "target_file": "pkg/api/secrets/ciphers/encryption.go",
  "test_file": "pkg/api/secrets/ciphers/encryption_test.go",
  "change_type": "bug_fix",
  "priority": "MUST",
  "summary": "Fix EncryptLargeString to chunk by bytes (OAEP byte limit) instead of runes; add UTF-8 regression tests",
  "key_constraint": "maxPlain = rsaKey.Size() - 2*sha256.Size - 2 (190 for 2048-bit); loop over []byte(s), not lo.ChunkString",
  "backward_compat": true,
  "decrypt_change_required": false,
  "next_character": "engineer",
  "delegations": []
}
```

---

## 9. Next Concrete Step

**Engineer:** implement the byte-chunked RSA loop above in `pkg/api/secrets/ciphers/encryption.go` and add the five regression tests listed in §5. Nothing else changes. Ship it.