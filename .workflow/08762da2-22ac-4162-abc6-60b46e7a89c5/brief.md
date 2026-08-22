Bug: `sc secrets add/hide` fails on UTF-8 secret files — "crypto/rsa: message too long for RSA key size"

## Summary

`sc secrets add` / `sc secrets hide` fail to encrypt a secret file when the file
contains enough **multi-byte UTF-8** characters (emoji, box-drawing `─`, arrows
`→ ↔`, em-dash `—`, Cyrillic, CJK, accented letters) clustered together. The
command aborts with:

```
Error executing command: failed to re-encrypt all secrets:
  failed to encrypt secret file: "<path>" with publicKey "ssh-rsa AAAAB3N":
  failed to encrypt secret: crypto/rsa: message too long for RSA key size
```

The failure is **data-dependent and intermittent**: an ASCII-only file of the
same size encrypts fine; a file with a dense run of multi-byte characters fails.
This makes it confusing to diagnose (it looks like a key-size problem, but all
keys are 2048-bit and the file is far smaller than any RSA limit).

## Impact

- Any secret file with non-ASCII content (comments with box-drawing/arrows,
  emoji in values, Cyrillic/CJK text) can become un-encryptable.
- Severity: medium. Blocks adding/rotating affected secrets; no data loss.
- Real case: a host_vars file with section-header comments built from `─` and
  `→ ↔ —` characters (worst 128-rune window = 260 bytes) failed; the same file
  with those replaced by ASCII (`-`, `->`, `<->`, `--`) encrypted fine.

## Root cause

`pkg/api/secrets/ciphers/encryption.go`, `EncryptLargeString` (≈ line 147):

```go
func EncryptLargeString(key crypto.PublicKey, s string) ([]string, error) {
    if rsaKey, ok := key.(*rsa.PublicKey); ok {
        chunks := lo.ChunkString(s, rsaKey.Size()/2)          // <-- (1) chunk size in RUNES
        for idx, chunk := range chunks {
            encryptedData, err := rsa.EncryptOAEP(
                sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)   // <-- (2) OAEP byte limit
            ...
        }
    }
}
```

Two compounding issues:

1. **Chunking is by runes, the limit is in bytes.** `lo.ChunkString` splits the
   string by **rune count** (it operates on `[]rune`). The chunk is then converted
   back with `[]byte(chunk)`. For multi-byte UTF-8, the byte length of a chunk can
   be up to 4× its rune count.

2. **The chunk size `rsaKey.Size()/2` is not a safe OAEP message size.**
   RSA-OAEP can encrypt at most `k − 2·hLen − 2` bytes, where `k` = modulus size
   in bytes and `hLen` = hash output size. With SHA-256 (`hLen = 32`) and a
   2048-bit key (`k = 256`): **max = 256 − 64 − 2 = 190 bytes**.
   The code uses `rsaKey.Size()/2 = 128` as the chunk size.

   - For **ASCII**: 128 runes = 128 bytes ≤ 190 → OK (which is why it usually works).
   - For **multi-byte UTF-8**: a 128-rune chunk can be 129…512 bytes. As soon as
     one chunk exceeds 190 bytes, `rsa.EncryptOAEP` returns
     `crypto/rsa: message too long for RSA key size`.

So `Size()/2` only *happens* to be safe for ASCII; it is unsafe in general
because (a) it counts runes not bytes and (b) `Size()/2` > the true OAEP limit
once content is multi-byte.

### Verification (math + measured)

- 2048-bit key: OAEP-SHA256 limit = 190 bytes; code chunk size = 128.
- Measured worst-case 128-rune window (bytes):
  - ASCII-heavy host_vars: 160 B, 168 B → encrypt OK.
  - Box-drawing-heavy host_vars: **260 B** → encrypt FAILS.

## Reproduction

```sh
# A file whose 128-rune window exceeds 190 bytes when UTF-8 encoded.
printf '# %s\n' "$(python3 -c 'print("─"*120)')" > /tmp/secret.txt   # 120x U+2500 = 360 bytes
sc secrets add /tmp/secret.txt
# -> crypto/rsa: message too long for RSA key size
```

## Proposed fix

Chunk by **bytes** with a size that respects the OAEP limit, and operate on the
byte slice instead of runes. Splitting a multi-byte character across chunk
boundaries is safe here because `DecryptLargeString` concatenates the decrypted
byte chunks back exactly (`strings.Join` of the per-chunk strings reproduces the
original byte sequence; Go strings are byte sequences, so an invalid-UTF-8
fragment in one chunk is harmless once joined).

```go
func EncryptLargeString(key crypto.PublicKey, s string) ([]string, error) {
    if rsaKey, ok := key.(*rsa.PublicKey); ok {
        // RSA-OAEP max message = k - 2*hLen - 2 (SHA-256 => hLen=32). Leave a
        // small safety margin.
        maxPlain := rsaKey.Size() - 2*sha256.Size() - 2   // 190 for 2048-bit
        if maxPlain <= 0 {
            return nil, errors.Errorf("RSA key too small (%d bits) for OAEP-SHA256", rsaKey.Size()*8)
        }
        data := []byte(s)
        res := make([]string, 0, (len(data)+maxPlain-1)/maxPlain)
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
        return res, nil
    }
    // ... ed25519 branch unchanged ...
}
```

`DecryptLargeString` needs no change (it already decodes + RSA-decrypts each
chunk and joins the resulting bytes).

### Backward compatibility

Existing secrets stored with the old rune-based chunking still decrypt correctly
(decryption is chunk-list driven and chunk-size agnostic). Only **re-encryption**
(add/hide) produces the new byte-based chunks. A `--force` re-encrypt of the whole
store after the fix is safe.

### Alternative (recommended longer term)

Switch the RSA path to **hybrid encryption**, mirroring the existing ed25519
path (`encryptWithEd25519`): generate a random symmetric key, encrypt the file
once with AES-GCM / ChaCha20-Poly1305, and RSA-OAEP-encrypt only the small
symmetric key. This eliminates chunking entirely, is faster, and removes this
whole class of size bug.

## Secondary note (latent inconsistency)

`EncryptWithPublicRSAKey` (same file, ≈ line 86) does a **single-shot**
`rsa.EncryptOAEP` with **SHA-512** (`hLen=64` → max 126 bytes for a 2048-bit key)
and no chunking, whereas `EncryptLargeString` uses **SHA-256** + chunking. If any
caller routes large data through `EncryptWithPublicRSAKey`, it will fail for
anything > 126 bytes. Consider standardizing the hash and always going through
the chunked/hybrid path.

## Affected file

- `pkg/api/secrets/ciphers/encryption.go` — `EncryptLargeString` (chunking),
  and `EncryptWithPublicRSAKey` (hash inconsistency).