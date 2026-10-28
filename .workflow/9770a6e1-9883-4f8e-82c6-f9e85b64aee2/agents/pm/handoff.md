# PM handoff — `agent-commits-fail-dco-on-api`

## 1. Commit + branch identity
1. **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/9770a6e1-9883-4f8e-82c6-f9e85b64aee2`
2. No code mutations in this turn.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|

## 3. Requirements and scope decisions
1. **MUST — Fix the real bug:** Change RSA chunking in `pkg/api/secrets/ciphers/encryption.go` from rune windows to byte slices bounded by `rsaKey.Size() - 2*sha256.Size() - 2`; reject a nonpositive bound. The live loop is in `EncryptLargeStringWithAAD`, which `EncryptLargeString` calls (source: repository file on this run’s branch).
2. **MUST — Preserve behavior:** Keep OAEP-SHA256, existing AAD chunk labels, chunk order, and the legacy nil-AAD path. Byte boundaries may split a UTF-8 rune; concatenating decrypted bytes must still reproduce the original input. Leave decryption unchanged unless a test reveals a necessary fix.
3. **MUST — Prove DCO on this run’s PR:** Deliver a source-changing PR in `api`; verify the `Signed-off-by:` trailer and agent identity on **every PR commit**, including engine-authored commits. Record the PR’s DCO-check verdict. Do not rewrite commits manually to manufacture the proof.
4. **MUST — Respect gates and resource limits:** Run only `go test ./pkg/api/secrets/...`; do not bypass branch protection. An open PR awaiting human reviews is an acceptable end state **if DCO is green**.
5. **NICE-TO-HAVE — None:** Do not spend this proof run on the separate SHA-512 single-shot helper.
6. **CUT —** Hybrid-encryption redesign, conductor/runtime sign-off implementation, GraphQL phase B, signing-key phase C, ruleset changes, and unrelated file edits.

## 4. Numbered acceptance criteria for downstream
1. A PR against `simple-container-com/api` contains a non-zero source diff under `pkg/api/secrets/ciphers/` that fixes RSA byte-bound chunking.
2. A regression test encrypts a multibyte input whose rune-based window exceeds the 190-byte OAEP-SHA256 limit for a 2048-bit key, then decrypts to byte-identical input. Cover the AAD path and retain legacy nil-AAD round-trip coverage.
3. `go test ./pkg/api/secrets/...` passes; no repository-wide build or test is run.
4. Every commit introduced by the PR has a `Signed-off-by:` trailer naming its agent identity, without manual commit rewrites. The PR’s DCO check is green; if unavailable, provide trailer evidence and check verdicts per commit SHA as specified in the brief.
5. The PR remains open if human-review or signature gates prevent merging; report those gates separately from the DCO result.

## 5. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Source-changing PR fixes byte chunking | FAIL — pending downstream | PM made no source edit or PR. Repository inspection confirms the live RSA loop still calls `lo.ChunkString(s, rsaKey.Size()/2)` in `EncryptLargeStringWithAAD`. |
| AC2 | Every PR commit signed off; DCO green or specified fallback evidence | FAIL — pending downstream | No PR or new commits to inspect in this turn. Developer and QA must check the eventual PR commit range and DCO result. |
| AC3 | Scoped tests pass, including multibyte regression and round-trip | FAIL — pending downstream | PM ran no tests; regression and scoped test execution belong to Developer and QA. |
| AC4 | Leave PR open if human-review gate blocks merge | FAIL — pending downstream | No PR exists yet; DevOps must report its final state without bypassing protection. |

## 6. Tests run
1. Not applicable — PM did not mutate code or run tests.

## 7. Architect handoff
1. Specify the smallest change to the **existing AAD-aware RSA loop**, including the nonpositive-bound error and byte-slice iteration; avoid altering the ed25519 path.
2. Give Developer a focused regression test and the sole permitted test command. Give QA and DevOps an explicit PR-commit trailer and DCO-check verification checklist. **Next step:** hand this scoped design to Developer.

**Verdict:** signoff