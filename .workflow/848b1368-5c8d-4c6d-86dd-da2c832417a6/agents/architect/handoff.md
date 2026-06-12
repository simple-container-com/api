Good — only three files in the ciphers package, no CLAUDE.md at root (404). I have the complete picture. Now I'll produce the architect handoff.

---

# Architect Handoff — `EncryptLargeString` UTF-8 Bug Fix

**Run:** `848b1368-5c8d-4c6d-86dd-da2c832417a6`
**Date:** 2026-06-12
**Author:** Dan Johnson (architect)
**Status:** Architecture reviewed — approved with one important correction to the PM spec

---

## 1. What I Verified Against Live Source

I pulled the actual files from the workflow branch before writing this. Nothing is assumed from the brief alone.

| File | Status |
|------|--------|
| `pkg/api/secrets/ciphers/encryption.go` | Confirmed — **bug is present**, unpatched |
| `pkg/api/secrets/ciphers/encryption_test.go` | Confirmed — no UTF-8 test cases |
| `pkg/api/secrets/ciphers/encryption_extra_test.go` | Confirmed — exists, also no UTF-8 cases |
| `CLAUDE.md` (repo root) | Not present — no special build constraints found |
| `go.mod` | `github.com/samber/lo v1.53.0` is a direct dependency |

---

## 2. Architecture Verdict on the PM Proposal

**Overall: Approved with one correction (§3) and one enhancement (§4).**

The PM's proposed fix is structurally correct. The byte-chunked loop replacing `lo.ChunkString` is the right approach and the OAEP math is right. I have no structural objection. My two notes below are not blocking the approach — one is a code correctness fix, one is a decrypt-side simplification that the engineer should absorb.

---

## 3. ⚠️ Correction: `sha256.Size` is a constant, not a function call

The PM spec writes:
```go
maxPlain := rsaKey.Size() - 2*sha256.Size - 2
```

**This is correct Go.** `sha256.Size` (from `crypto/sha256`) is an untyped integer constant `32`. No function call needed — no `()`. The PM spec is right; I'm confirming it explicitly because the brief's alternative fix snippet writes `sha256.Size()` with parens (wrong) while the PM spec drops the parens (right). Engineer: use `sha256.Size` (no parens).

---

## 4. Decrypt-Side Observation: `lo` import stays (no removal needed)

The PM says "verify and drop the `lo` import if unused." After reading the live source, **`lo` is still used in `DecryptLargeString`**:

```go
return []byte(strings.Join(lo.Map(decrChunks, func(chunk []byte, _ int) string {
    return string(chunk)
}), "")), nil
```

So **the `lo` import must remain**. The fix only touches `EncryptLargeString`. No import changes are needed.

However, I want to flag a subtlety here that the engineer should be aware of:

> `DecryptLargeString` joins decrypted byte chunks via `string(chunk)` + `strings.Join`. This is fine. Go strings are byte sequences. A chunk that contains a split-in-the-middle UTF-8 codepoint will have its bytes faithfully preserved and re-joined. The original string is reconstructed correctly at the byte level, which is what matters. No change to decrypt needed.

---

## 5. Precise Change Specification

### 5.1 File: `pkg/api/secrets/ciphers/encryption.go`

**Replace the RSA branch of `EncryptLargeString` (lines ~147–156 in live source):**

```go
// BEFORE
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

```go
// AFTER
// RSA-OAEP max plaintext = k − 2·hLen − 2.
// SHA-256: hLen=32. For a 2048-bit key (k=256): 256 − 64 − 2 = 190 bytes.
// sha256.Size is the package constant 32; no function call.
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

**The `lo` import line stays unchanged** (still needed by `DecryptLargeString`).

**No other changes to this file.**

### 5.2 File: `pkg/api/secrets/ciphers/encryption_test.go` (or `encryption_extra_test.go`)

Add the following five tests. I recommend adding to `encryption_extra_test.go` since it's the newer, more organised file, but either works.

```go
func TestEncryptLargeString_UTF8_BoxDrawing(t *testing.T) {
    RegisterTestingT(t)
    privKey, pubKey, err := GenerateKeyPair(2048)
    Expect(err).ToNot(HaveOccurred())
    // ─ is U+2500, 3 bytes each; 256 runes = 768 bytes — well over old 128-rune chunk limit
    input := strings.Repeat("─", 256)
    chunks, err := EncryptLargeString(pubKey, input)
    Expect(err).ToNot(HaveOccurred())
    dec, err := DecryptLargeString(privKey, chunks)
    Expect(err).ToNot(HaveOccurred())
    Expect(string(dec)).To(Equal(input))
}

func TestEncryptLargeString_UTF8_Emoji(t *testing.T) {
    RegisterTestingT(t)
    privKey, pubKey, err := GenerateKeyPair(2048)
    Expect(err).ToNot(HaveOccurred())
    // 🔑 is U+1F511, 4 bytes each; 100 runes = 400 bytes
    input := strings.Repeat("🔑", 100)
    chunks, err := EncryptLargeString(pubKey, input)
    Expect(err).ToNot(HaveOccurred())
    dec, err := DecryptLargeString(privKey, chunks)
    Expect(err).ToNot(HaveOccurred())
    Expect(string(dec)).To(Equal(input))
}

func TestEncryptLargeString_UTF8_Mixed(t *testing.T) {
    RegisterTestingT(t)
    privKey, pubKey, err := GenerateKeyPair(2048)
    Expect(err).ToNot(HaveOccurred())
    // Real-world-style header with box-drawing, arrows, em-dash, Cyrillic
    input := "─────────────────────────\n" +
        "→ ↔ — Привет мир\n" +
        strings.Repeat("─", 80) + "\n" +
        "value: тест\n"
    chunks, err := EncryptLargeString(pubKey, input)
    Expect(err).ToNot(HaveOccurred())
    dec, err := DecryptLargeString(privKey, chunks)
    Expect(err).ToNot(HaveOccurred())
    Expect(string(dec)).To(Equal(input))
}

func TestEncryptLargeString_UTF8_ChunkBoundaryMidRune(t *testing.T) {
    RegisterTestingT(t)
    privKey, pubKey, err := GenerateKeyPair(2048)
    Expect(err).ToNot(HaveOccurred())
    // Construct input whose byte length forces a chunk boundary to fall
    // in the middle of a 3-byte rune. 190 bytes = 63 × "─" (63×3=189) + 1 byte
    // of a second ─ (which starts a 3-byte seq). The fix must handle this safely.
    // Total: use 191 bytes of 3-byte chars = 63 full + partial → two chunks.
    // Simplest: repeat ─ 64 times (192 bytes). Boundary at 190 = mid-rune 64.
    input := strings.Repeat("─", 64) // 192 bytes; chunk 1 = bytes [0:190], chunk 2 = bytes [190:192]
    chunks, err := EncryptLargeString(pubKey, input)
    Expect(err).ToNot(HaveOccurred())
    Expect(len(chunks)).To(Equal(2)) // sanity: verify we got 2 chunks
    dec, err := DecryptLargeString(privKey, chunks)
    Expect(err).ToNot(HaveOccurred())
    Expect(string(dec)).To(Equal(input))
}

func TestEncryptLargeString_SmallKey_TooSmall(t *testing.T) {
    RegisterTestingT(t)
    // A 512-bit key: k=64, maxPlain = 64 − 64 − 2 = −2 → guard fires.
    privKey, err := rsa.GenerateKey(rand.Reader, 512)
    Expect(err).ToNot(HaveOccurred())
    _, err = EncryptLargeString(&privKey.PublicKey, "any input")
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("RSA key too small"))
}
```

**Import additions needed for the test file:**
- `"crypto/rand"` (for the 512-bit key test)
- `"crypto/rsa"` (for the 512-bit key test)
- `"strings"` (already present in `encryption_extra_test.go`)

---

## 6. Architecture Decision Record

### ADR-001: Chunk by bytes, not runes

**Decision:** Replace `lo.ChunkString(s, rsaKey.Size()/2)` with a byte-slice loop using `maxPlain = rsaKey.Size() - 2*sha256.Size - 2`.

**Why:**
- OAEP's limit is defined in bytes. Chunking by rune count is a type mismatch with the cryptographic constraint.
- `rsaKey.Size()/2 = 128` (for 2048-bit) is numerically less than the OAEP limit of 190 bytes, but that gap is consumed by multi-byte runes — up to 4× inflation.
- Byte-level splitting at a codepoint boundary is safe because `DecryptLargeString` reconstructs at the byte level too (`string(chunk)` on a byte slice, then `strings.Join`).

**Alternatives considered:**
1. Chunk by rune but shrink to `maxPlain/4` (48 bytes for 2048-bit) — conservative but wasteful (4× more RSA operations). Rejected.
2. Full hybrid encryption (AES-GCM + RSA-OAEP on sym key) — correct long-term, explicitly out of scope per PM decision.

**Backward compatibility:** Decryption is chunk-count-driven. Old rune-chunked ciphertext still decrypts correctly through the unmodified `DecryptLargeString`.

---

## 7. Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Test for `TestEncryptLargeString_SmallKey_TooSmall` depends on 512-bit key generation succeeding | Low — Go stdlib allows it, just slow | Acceptable; test is deterministic |
| `TestEncryptLargeString_UTF8_ChunkBoundaryMidRune` assertion `len(chunks) == 2` fails if test input changes | Low | Assertion documents intent; easy to fix if input changes |
| `lo` import accidentally removed during refactor | Low | Explicitly called out in §4; `DecryptLargeString` still uses it |
| Linter (`golangci-lint`) complains about magic number `2` in `maxPlain` formula | Low | Add comment explaining the OAEP formula — already in proposed code |

---

## 8. What Is NOT Changing (confirmed cut)

- `DecryptLargeString` — no change, backward-compat by design
- `EncryptWithPublicRSAKey` — SHA-512 inconsistency is a known latent issue, deferred
- `lo` import — **stays** (still used in decrypt path)
- Hybrid encryption migration — explicitly out of scope
- No swagger regen (zero API surface change)
- No `go.mod` change (no new dependencies)

---

## 9. Verification Gate for Engineer

Before handing off, the engineer **must** confirm all three:

1. `git log -1 --format="%H %s" -- pkg/api/secrets/ciphers/encryption.go` shows their commit.
2. `git log -1 --format="%H %s" -- pkg/api/secrets/ciphers/encryption_extra_test.go` (or `encryption_test.go`) shows their commit.
3. `go test ./pkg/api/secrets/ciphers/... -v -count=1` is green including all five new tests.

---

## 10. Handoff JSON

```json
{
  "run_id": "848b1368-5c8d-4c6d-86dd-da2c832417a6",
  "character": "architect",
  "date": "2026-06-12",
  "approved": true,
  "corrections_to_pm_spec": [
    "sha256.Size has no parens (it is a constant, not a function) — PM spec was already correct; brief's alternative had parens — do NOT add parens",
    "lo import must NOT be removed — DecryptLargeString still uses lo.Map"
  ],
  "target_file": "pkg/api/secrets/ciphers/encryption.go",
  "test_target": "pkg/api/secrets/ciphers/encryption_extra_test.go",
  "change_type": "bug_fix",
  "imports_changed": false,
  "lo_import_keep": true,
  "maxPlain_formula": "rsaKey.Size() - 2*sha256.Size - 2",
  "sha256_size_note": "sha256.Size is an untyped constant 32, no parens",
  "backward_compat": true,
  "decrypt_change_required": false,
  "new_test_count": 5,
  "next_character": "engineer"
}
```

---

## 11. Next Concrete Step

**Engineer:** The fix is surgical. Two files, ~15 lines changed, five new tests. The architecture is locked — don't deviate from the byte-chunked approach. Specific callouts:

1. In `encryption.go`, replace the four-line rune-chunk loop with the byte-loop above. Keep `lo` import.
2. Add the five test functions from §5.2 to `encryption_extra_test.go`. You'll need `crypto/rand` and `crypto/rsa` imports added to that file.
3. Run `go test ./pkg/api/secrets/ciphers/... -v` — all tests must be green before committing.
4. Run the git log verification gate (§9) before handing off.