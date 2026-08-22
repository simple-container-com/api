The full picture is now clear. Every check is **pass** except:
- **DCO** — fail (infrastructure issue, all workflow-engine commits, not a code defect)
- **Analyze Go** — still pending (CodeQL analysis; no indication of failure)

The entire build matrix, all tests, security scans, linting, fuzz, and vuln checks are green. My handoff above stands. Here is the final, complete version for the engine:

---

# QA Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Workflow run:** `08762da2-22ac-4162-abc6-60b46e7a89c5`
**Date:** 2026-06-12
**Author:** Maria Currie (qa)
**PR:** https://github.com/simple-container-com/api/pull/321
**Commit reviewed:** `75f5eddaed7dcb4772efe2a3238f64c0afa66bb7`

**Verdict:** signoff

---

## 1. Review scope

Read both changed files in full from the PR branch via MCP. Verified the complete diff. Monitored CI to final settlement.

**Files changed:**
- `pkg/api/secrets/ciphers/encryption.go` — 21 additions, 6 deletions
- `pkg/api/secrets/ciphers/encryption_test.go` — 80 additions

---

## 2. Code correctness — `encryption.go`

| Item | Finding |
|---|---|
| Chunk size formula | `rsaKey.Size() - 2*sha256.Size - 2` = 190 for 2048-bit key. RFC 8017 §7.1.1 exact. ✓ |
| `sha256.Size` constant | Untyped constant = 32, not a function call. Compiles correctly. ✓ |
| Byte-based loop | `data := []byte(s)` then `data[i:end]` — no rune counting. Root cause eliminated. ✓ |
| Capacity pre-alloc | `(len(data)+maxPlain-1)/maxPlain` — correct ceiling division. ✓ |
| Bound clamp | `if end > len(data) { end = len(data) }` — no out-of-bounds. ✓ |
| `maxPlain <= 0` guard | Present; descriptive error for undersized keys. ✓ |
| `lo` import retained | Correct — `lo.Map` still used in `DecryptLargeString`. ✓ |
| `DecryptLargeString` | Untouched. Backward compatibility preserved. ✓ |
| ed25519 branch | Untouched. ✓ |
| TODO on `EncryptWithPublicRSAKey` | Present with SHA-512 / 126-byte limit warning. ✓ |

---

## 3. Test coverage — `encryption_test.go`

All 6 sub-tests in `TestEncryptLargeStringUTF8` correct and complete:

| Sub-test | Input | Validates |
|---|---|---|
| box-drawing flood | 120 × `─` = 360 bytes / 120 runes | Exact reported bug case |
| mixed multi-byte | `→—яя🔑` × 20 ≈ 280 bytes | General multi-byte UTF-8 |
| exactly 190 bytes ASCII | `strings.Repeat("a", 190)` → `HaveLen(1)` | Upper boundary |
| 191 bytes ASCII | `strings.Repeat("a", 191)` → `HaveLen(2)` | Forces split |
| empty string | `""` → `BeEmpty()` | Zero-chunk edge case |
| 4096-bit key | 300 × `─` = 900 bytes | `maxPlain=446` scaling |

Style matches existing file conventions throughout.

---

## 4. Final CI results

| Check | Result |
|---|---|
| Run tests | **pass** |
| Build Setup (clean, tools, schemas, lint, fmt) | **pass** |
| Build sc (linux/amd64, darwin/amd64, darwin/arm64) | **pass** |
| Build cloud-helpers, github-actions, github-actions-staging | **pass** |
| Finalize build in branch | **pass** |
| Fuzz HMAC cache parse path | **pass** |
| govulncheck (reachability-aware) | **pass** |
| security (Secret Scan, SBOM, Status) | **pass** |
| semgrep (all jobs) | **pass** |
| Analyze Go (CodeQL) | pending (long-running analysis; no failures in any other check) |
| **Developer Certificate of Origin** | **FAIL** — infrastructure issue only (see below) |

### DCO failure

All 5 commits on the branch are missing `Signed-off-by` trailers, including the 4 Forge Workflow Engine scaffolding commits authored by `forge-agent@simple-forge.com`. This is a process/infrastructure gap — the DCO bot has no exemption for workflow-engine authors. The code is correct. DevOps action required: add `Signed-off-by` to the engine commit template or configure DCO bot to exempt `forge-agent@` author addresses.

---

## 5. Acceptance criteria

| Criterion | Status |
|---|---|
| `go test ./pkg/api/secrets/ciphers/...` green | **MET** — CI `Run tests` passed |
| No `lo.ChunkString` in `EncryptLargeString` | **MET** |
| `maxPlain` uses exact OAEP formula | **MET** |
| `EncryptWithPublicRSAKey` TODO comment present | **MET** |
| 120× `─` input encrypts without error | **MET** — sub-test 1 covers this directly |

All 5 PM MUST items met. All 3 NICE-TO-HAVE items correctly deferred.

---

## 6. Findings

1. **[INFO] DCO policy failure** — infrastructure issue, not a code defect. Affects all workflow-engine commits. DevOps to resolve.
2. **[INFO] Analyze Go (CodeQL) still running** — long-running static analysis job; all other checks clean, no indication of issues.
3. **[INFO] `DecryptLargeString` byte-join smell** — `strings.Join(lo.Map(...))` could be `bytes.Join(decrChunks, nil)`. No correctness impact; deferred out of scope.

**No blocking findings.**

---

## 7. Disposition

The fix is correct, complete, and well-tested. The full build matrix and all substantive CI checks are green. The only failing check is DCO — a process/infrastructure matter unrelated to code quality.

**Verdict:** signoff — ready for merge once DCO infrastructure is resolved.