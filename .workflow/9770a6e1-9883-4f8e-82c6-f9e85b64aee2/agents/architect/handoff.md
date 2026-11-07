# Architect handoff — `agent-commits-fail-dco-on-api`

## 1. Commit + branch identity

No code mutations in this turn.

Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/9770a6e1-9883-4f8e-82c6-f9e85b64aee2`

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---:|---|

## 3. Architecture plan for Developer

### Structural concern

The bug is not cryptographic design; it is a boundary-unit mismatch:

```text
current chunking unit: runes
RSA-OAEP limit unit: bytes
```

For a 2048-bit RSA key with OAEP-SHA256:

```text
maxPlain = rsaKey.Size() - 2*sha256.Size() - 2
         = 256 - 64 - 2
         = 190 bytes
```

The current implementation in `pkg/api/secrets/ciphers/encryption.go` chunks RSA plaintext with `lo.ChunkString(s, rsaKey.Size()/2)`, i.e. 128 runes. A 128-rune window can exceed 190 bytes for dense UTF-8 input, causing `crypto/rsa: message too long for RSA key size`.

### Minimal implementation

In `pkg/api/secrets/ciphers/encryption.go`, update only the RSA branch of `EncryptLargeStringWithAAD`:

1. Remove `lo.ChunkString(s, rsaKey.Size()/2)` from the RSA encryption path.
2. Compute the OAEP-SHA256 byte budget:

   ```go
   maxPlain := rsaKey.Size() - 2*sha256.Size - 2
   if maxPlain <= 0 {
       return nil, errors.Errorf("RSA key too small for OAEP-SHA256: max plaintext size is %d bytes", maxPlain)
   }
   ```

   Why: the encryption helper should fail deterministically with a useful error if the RSA modulus is too small for SHA-256 OAEP overhead.

3. Convert once to bytes:

   ```go
   plaintext := []byte(s)
   ```

4. Slice by byte offsets:

   ```go
   n := (len(plaintext) + maxPlain - 1) / maxPlain
   res = make([]string, n)
   for idx, start := 0, 0; start < len(plaintext); idx, start = idx+1, start+maxPlain {
       end := start + maxPlain
       if end > len(plaintext) {
           end = len(plaintext)
       }

       encryptedData, err := rsa.EncryptOAEP(
           sha256.New(),
           rand.Reader,
           rsaKey,
           plaintext[start:end],
           chunkLabel(aad, idx, n),
       )
       if err != nil {
           return nil, errors.Wrapf(err, "failed to encrypt secret")
       }
       res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
   }
   ```

5. Preserve current empty-string behavior. With the loop above, `""` produces `[]string{}` just like `lo.ChunkString("", ...)` currently does. `DecryptLargeStringWithAAD` already preserves legacy nil-AAD empty behavior and rejects empty AAD-bound ciphertext lists, so no decryption change is needed.

### Design boundaries — do not expand scope

- Do **not** redesign to hybrid AES-GCM.
- Do **not** modify ed25519/X25519 paths.
- Do **not** change AAD label semantics.
- Do **not** standardize `EncryptWithPublicRSAKey` SHA-512 vs SHA-256 in this run. It is a separate single-shot helper, not the `sc secrets add/hide` large-string path under test.
- Do **not** touch files outside `pkg/api/secrets/ciphers/` unless a compile failure proves it is necessary.

### Regression test shape

Add a focused test under `pkg/api/secrets/ciphers/`, preferably in `aad_test.go` because the live path is `EncryptLargeStringWithAAD`, or in `encryption_test.go` if Developer wants to keep all RSA large-string tests together.

Suggested test intent:

```go
func TestEncryptLargeStringWithAAD_RSAChunksByBytesForMultibyteInput(t *testing.T) {
    RegisterTestingT(t)

    priv, pub, err := GenerateKeyPair(2048)
    Expect(err).NotTo(HaveOccurred())

    aad := []byte("sc-scope-v1\x00prod\x00dense-utf8")

    // 128 runes of "界" is 384 bytes, which exceeds the 190-byte
    // OAEP-SHA256 plaintext limit for a 2048-bit RSA key. The old
    // rune-chunking implementation fails with "message too long".
    input := strings.Repeat("界", 128) + "tail"

    chunks, err := EncryptLargeStringWithAAD(pub, input, aad)
    Expect(err).NotTo(HaveOccurred())
    Expect(len(chunks)).To(BeNumerically(">", 1))

    got, err := DecryptLargeStringWithAAD(priv, chunks, aad)
    Expect(err).NotTo(HaveOccurred())
    Expect(got).To(Equal([]byte(input)))
}
```

Also retain/verify legacy nil-AAD coverage already exists:
- `TestAAD_NilRoundTrip_LegacyCompatible` in `pkg/api/secrets/ciphers/aad_test.go`
- `TestRSAEncryptionDecryption/RSA encrypt/decrypt large string` in `pkg/api/secrets/ciphers/encryption_test.go`
- `TestEncryptLargeString_RoundTrip` in `pkg/api/secrets/ciphers/encryption_extra_test.go`

Developer may add a second nil-AAD dense UTF-8 case if the first AAD regression feels too narrow, but keep it small.

### DCO proof checklist for QA/DevOps

The code fix is only half the slice. The proof objective is DCO on a real source-changing PR.

Downstream must verify:

1. PR has a non-zero diff outside `.workflow/`, specifically under `pkg/api/secrets/ciphers/`.
2. Every commit on the PR branch has a `Signed-off-by:` trailer.
3. The source-changing commit also has a `Signed-off-by:` trailer; do not rely only on engine metadata commits.
4. DCO check is green on the PR. If GitHub does not run it because of approval policy, capture commit-by-commit trailer evidence and per-commit check verdicts where available.
5. If the PR remains open due to human-review/code-owner/required-signature gates, report that as expected and do not bypass branch protection.

Useful commands for the downstream clone, with real output to be captured by the role that runs them:

```bash
git log --format='%h %an <%ae>%n%B%n---END---' origin/main..HEAD
go test ./pkg/api/secrets/...
```

Do not run `go test ./...`.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | PR contains a non-zero source diff under `pkg/api/secrets/ciphers/` fixing RSA byte-bound chunking | FAIL — pending Developer/DevOps | Architect made no source mutation or PR. Repository inspection on this branch shows the current RSA path in `pkg/api/secrets/ciphers/encryption.go` still calls `lo.ChunkString(s, rsaKey.Size()/2)` inside `EncryptLargeStringWithAAD`. |
| AC2 | Regression test covers multibyte input whose rune window exceeds the 190-byte OAEP-SHA256 limit and round-trips with AAD/nil-AAD behavior retained | FAIL — pending Developer | Existing tests observed include `TestAAD_NilRoundTrip_LegacyCompatible`, `TestRSAChunkFraming_ReorderTruncateSpliceFails`, and `TestEncryptLargeString_RoundTrip`, but no dense UTF-8 byte-vs-rune regression was added by this architect turn. |
| AC3 | `go test ./pkg/api/secrets/...` passes; no repo-wide build/test | FAIL — pending Developer/QA | Architect ran no tests and made no mutations. Downstream must run exactly `go test ./pkg/api/secrets/...`. |
| AC4 | Every PR commit has `Signed-off-by:` trailer and DCO check is green or equivalent commit/check evidence is recorded | FAIL — pending QA/DevOps | No PR exists from this architect turn. Downstream must inspect actual PR commits; do not fabricate SHA/trailer evidence. |
| AC5 | PR remains open if human-review/signature gates prevent merge; gates reported separately from DCO | FAIL — pending DevOps | No PR exists yet. DevOps must leave PR open if only human gates block merge and report that separately from DCO status. |

## 5. Tests run

Not applicable — architect turn only; no files were mutated and no tests were run.

**Verdict:** signoff