Both files committed. All gates pass.

---

# Developer Handoff — `EncryptLargeString` UTF-8 Bug Fix

**Run:** `848b1368-5c8d-4c6d-86dd-da2c832417a6`
**Date:** 2026-06-12
**Author:** David Black (developer)
**Status:** Shipped — PR open, all tests green

---

## What Was Done

### Bug Fix: `pkg/api/secrets/ciphers/encryption.go`

Replaced the rune-based `lo.ChunkString` loop in `EncryptLargeString`'s RSA branch with a byte-slice loop sized to the exact OAEP limit:

```go
// RSA-OAEP max plaintext = k − 2·hLen − 2.
// SHA-256: hLen=32 (sha256.Size). For a 2048-bit key (k=256): 256 − 64 − 2 = 190 bytes.
maxPlain := rsaKey.Size() - 2*sha256.Size - 2
if maxPlain <= 0 {
    return nil, errors.Errorf("RSA key too small (%d bits) for OAEP-SHA256", rsaKey.Size()*8)
}
data := []byte(s)
res = make([]string, 0, (len(data)+maxPlain-1)/maxPlain)
for i := 0; i < len(data); i += maxPlain {
    end := i + maxPlain
    if end > len(data) { end = len(data) }
    enc, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, data[i:end], nil)
    ...
}
```

Key decisions matching the architect spec:
- `sha256.Size` — no parens; it is the untyped constant `32`
- `lo` import **kept** — still used by `DecryptLargeString` (`lo.Map`)
- `DecryptLargeString` — **unchanged**; reconstructs at byte level so mid-rune splits are safe

### Tests: `pkg/api/secrets/ciphers/encryption_extra_test.go`

Added 5 regression tests. One adaptation from the architect spec: `TestEncryptLargeString_SmallKey_TooSmall` uses a synthetic `*rsa.PublicKey` with a 512-bit `big.Int` modulus instead of `rsa.GenerateKey(rand.Reader, 512)` — the current Go stdlib rejects 512-bit key generation with "crypto/rsa: 512-bit keys are insecure". The guard logic is identical; we just construct the key struct directly.

| Test | Result |
|------|--------|
| `TestEncryptLargeString_UTF8_BoxDrawing` (256× `─`, 768 bytes) | PASS |
| `TestEncryptLargeString_UTF8_Emoji` (100× `🔑`, 400 bytes) | PASS |
| `TestEncryptLargeString_UTF8_Mixed` (box-drawing + Cyrillic + arrows) | PASS |
| `TestEncryptLargeString_UTF8_ChunkBoundaryMidRune` (64× `─`, 192 bytes, 2 chunks) | PASS |
| `TestEncryptLargeString_SmallKey_TooSmall` (synthetic 512-bit modulus → guard) | PASS |

**Total: 32/32 tests pass** (`go test ./pkg/api/secrets/ciphers/... -v -count=1`)

---

## Verification Gate

```
git log -1 --format="%H %s" -- pkg/api/secrets/ciphers/encryption.go
b60fcd9 fix(secrets): chunk EncryptLargeString by bytes not runes to prevent RSA-OAEP overflow on UTF-8 files

git log -1 --format="%H %s" -- pkg/api/secrets/ciphers/encryption_extra_test.go
b60fcd9 fix(secrets): chunk EncryptLargeString by bytes not runes to prevent RSA-OAEP overflow on UTF-8 files
```

---

## Pull Request

**PR:** https://github.com/simple-container-com/api/pull/320
**Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/848b1368-5c8d-4c6d-86dd-da2c832417a6`
**Base:** `main`

**PULL_REQUEST:** https://github.com/simple-container-com/api/pull/320

---

## What Was NOT Changed (per scope)

- `DecryptLargeString` — no change needed; backward-compatible by design
- `EncryptWithPublicRSAKey` — SHA-512 hash inconsistency deferred (latent, no active caller affected)
- `go.mod` — no new dependencies; `lo` import retained
- No swagger regen — zero API surface change