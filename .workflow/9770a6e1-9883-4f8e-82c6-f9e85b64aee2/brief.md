[agent-commits-fail-dco-on-api] Agent commits carry no `Signed-off-by`, so no SDLC run can merge into `api`

Goal: prove phase (A) of this roadmap item — that a fresh Main Forge SDLC run dispatched against the `api` repo produces a real source-changing PR whose every commit carries a `Signed-off-by:` trailer and clears the repo's DCO check WITHOUT manual rewrites. The trailer-stamping code already shipped and is live: forge-conductor `cdc71712ab` (commitIdentity stamps both identity branches, merged PR #567) and forge-runtime `8a23d15ce2` (agent-worker prepare-commit-msg hook at clone time plus `-s` on the worker's own commit, PR #56). This run's job is NOT to re-implement those — it is to produce a small, genuine source change in THIS repo (`simple-container-com/api`) and demonstrate the DCO check green on the resulting PR.

Why this matters: `api` enforces DCO on every commit via `.github`; ruleset "Protect main branch" (rulesets/9144757) also requires 2 human approving reviews, code-owner review, and `required_signatures`. Previously (PR #431, run 77f30cc6) all 7 engine-authored commits failed DCO and the operator had to rewrite them by hand with `rebase --exec` + SSH signature. Phase (A) removes the DCO failure — the one gate that is Forge's own doing.

Scope — a small, real, defensible source change in `api`:
- Implement the fix for `sc secrets add/hide` UTF-8 chunking bug (issue #318) in `pkg/api/secrets/ciphers/encryption.go`: `EncryptLargeString` chunks by RUNES (`lo.ChunkString` with `rsaKey.Size()/2`) but the RSA-OAEP-SHA256 limit is BYTES (`k - 2*hLen - 2` = 190 bytes for a 2048-bit key), so a dense multi-byte window (worst 128-rune window measured 260 bytes) fails with `crypto/rsa: message too long for RSA key size`. Fix: chunk by bytes with `maxPlain := rsaKey.Size() - 2*sha256.Size() - 2`, guard `maxPlain <= 0`, encrypt each byte slice with `rsa.EncryptOAEP(sha256.New(), ...)`. `DecryptLargeString` needs no change (already chunk-list driven, chunk-size agnostic). Optionally standardize `EncryptWithPublicRSAKey`'s SHA-512 single-shot inconsistency only if trivially safe; do NOT redesign into hybrid encryption — that is explicitly out of scope.
- Keep the change MINIMAL — this run exists to exercise the DCO path, not to refactor. Do not touch anything outside `pkg/api/secrets/ciphers/` unless a compile dependency forces it.

Hard constraints:
1. EVERY commit on the run branch — engine bookkeeping commits AND the source commits — must carry a `Signed-off-by:` trailer naming the agent identity. If any commit lands without one, the proof FAILS, even if the code is perfect.
2. Do NOT bypass branch protection: no `--admin` merge, no ruleset edits. If the 2-human-review gate keeps the PR open, that is the expected outcome of this slice — report it, do NOT treat it as failure of the DCO proof. The proof is the GREEN DCO CHECK on the open PR, not the merge.
3. Build scope: run the tests ONLY for the touched package (`go test ./pkg/api/secrets/...`), never `go test ./...` on this repo — a full `go build ./...` can reach ~11.7 GB RSS and be OOM-killed in this microVM guest (12 GB). See roadmap item api-repo-slices-oom-the-guest. If a build command is killed, retry scoped to the single package, and do NOT escalate an OOM to a failure verdict without first having retried scoped.
4. Do NOT re-implement the conductor/runtime sign-off changes; they are merged and deployed. Verify only that the commits this run produces carry the trailer.

Definition of done:
- A PR in `simple-container-com/api` with at least one real source commit (non-zero diff outside `.workflow/`) fixing #318's chunking bug.
- Every commit on that PR carries `Signed-off-by:` and the DCO check on the PR is green (or, if the check cannot run on an unapproved PR, commit-by-commit trailer evidence + the check's verdict on each commit SHA).
- Unit tests for the touched package pass, byte-vs-rune chunking covered (a >190-byte multi-byte window encrypts; round-trip decrypt reproduces input).
- If the 2-human-review ruleset prevents merge, leave the PR OPEN and state that in the handoff — that is a successful DCO proof on an operator-mergeable PR.

Out of scope: GraphQL `createCommitOnBranch` (phase B), Forge-bot SSH signing key (phase C), hybrid AES-GCM encryption redesign, any change to the ruleset.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All seven driver steps are complete and the lint is clean. Final handoff:

# Roadmap Driver run 50663a54 — reconcile, escalate, re-dispatch ×2

## 1. Commit + branch identity
Branch: `«branch-from-triggering-run-redacted»`. Roadmap edits are committed by the workflow engine at turn end (Driver runs never push their own commits; no `Commit SHA:` line by spec).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +2/-2 | `agent-commits-fail-dco-on-api` row: 🚧 → ⬜ REDISPATCH → 🚧, repo cell forge-conductor → api, gate rewritten; `adhoc-workflow-no-parent-merge-policy` row: MANUAL → yes, ⬜ → 🚧, gate rewritten |
| docs/roadmap/items/agent-commits-fail-dco-on-api.md | +17/-2 | Reconcile record (escalated run 116fbf81, evidence checked), repo-routing correction forge-conductor → api, re-dispatch record, architect note that the live chunking loop is in `EncryptLargeStringWithAAD` not `EncryptLargeString`; `dispatched: "2026-10-12"`, `updated:` 2026-10-12 |
| docs/roadmap/items/adhoc-workflow-no-parent-merge-policy.md | +7/-5 | `dispatch: manual` → `auto` (design decision ≠ any of the three STEP-0 exceptions; recorded in History), dispatch record, `dispatched: "2026-10-12"`, `updated:` 2026-10-12 |
| docs/roadmap/driver-log/2026-10-12.md | +11/-0 | Full run note: intake, reconcile evidence, archive gate, picks, dispatch IDs, escalation |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | RECONCILE 🚧 IN PROGRESS slices (flip to ✅ if PR merged) | PASS | 25 🚧 rows; 24 carry empty `dispatched:` (part-built milestones — nothing to reconcile). The one live dispatch, `agent-commits-fail-dco-on-api`, checked against forge-conductor closed+open PRs (token match, 30 each), default-branch commits, and `list_runs`: no merged PR, no open PR, no source commit; terminal run `116fbf81` escalated 2026-10-12T00:45:30Z (>4h) at qa→devops — QA verdict failure, no api source commit. No slice qualified for ✅ SHIPPED. |
| AC2 | PICK the next un-done slice | PASS | Two slices, different repos: `agent-commits-fail-dco-on-api` (REDISPATCH priority, api) and `adhoc-workflow-no-parent-merge-policy` (NEXT, forge-conductor — its `manual` stamp met none of the three exceptions; corrected to `auto` and flagged in the log). All other ⬜/🟡 rows are genuinely manual-gated (operator decisions, live Meet/call/browser gates). Parallel cap respected (N=0 after reconcile). |
| AC3 | DISPATCH the Main Forge SDLC | PASS | `trigger_workflow` → workflow `99171cbe-7c27-4976-a999-931b834b5d62`, `queued:true` twice. Repo IDs verified character-exact: api = `6a1df1a2e639099d50d23c32`, forge-conductor = `6a1d87679aa5469478145de4` (the `...cc0c` umbrella id was NOT used). Dispatch-guard checked pre-dispatch: no non-terminal run held either slice. |
| AC4 | MARK the picked slice IN PROGRESS | PASS | Both item files now `status: in-progress`, `dispatched: "2026-10-12"`; both README rows 🚧; History lines added; lint confirms index↔item agreement. |

## 4. Tests run
- `bash docs/roadmap/lint.sh` → `372 items, 372 index rows, README.md 212 lines — 0 errors, 0 warnings`
- `git diff --check` → clean; working tree contains exactly the 4 files above.

## Operator items surfaced this run
1. **Escalation sent (Telegram, delivered):** `[agent-commits-fail-dco-on-api]` stale 4h+ — prior run escalated because it was routed to forge-conductor while its proof requires a source-changing run in `api`; re-queued with corrected repo.
2. Both dispatched SDLC runs fire **when this Driver run completes successfully** (trigger semantics) — reconcile them on the next Driver pass.
3. The api dispatch carries explicit OOM guardrails (scoped `go test ./pkg/api/secrets/...` only) and a no-branch-protection-bypass rule; per [api-repo-branch-protection-blocks-sdlc-automerge] the proof PR will need operator review to merge — that is expected, not a failure.

**Verdict:** signoff
workflowTriggerParentRunId: 50663a54-1863-43fd-8689-47c63ee2cd50
workflowTriggerRootRunId: 50663a54-1863-43fd-8689-47c63ee2cd50