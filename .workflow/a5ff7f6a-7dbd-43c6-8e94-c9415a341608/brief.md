[sc-stack-outputs-leaks-secrets-in-clear] sc stack outputs prints secret-marked outputs in the clear, with no flag to mask them

## Context
`sc stack outputs -s <stack>` prints every output value in the clear, including outputs that are secret-marked by the resource. A cluster-admin kubeconfig and a k3s join token were both observed unmasked in a live smoke's terminal output. `--show-secrets` controls *whether* secret outputs are shown at all, but once shown, they're always plaintext — there is no mask-by-default behavior.

## Scope (mask-by-default)
1. Mask secret-marked outputs by default (fixed-width placeholder like `<secret:redacted>`).
2. Add an explicit opt-in flag to reveal plaintext (reuse `--show-secrets` semantics or add `--reveal-secrets`), mirroring the existing `sc secrets reveal`/`sc secrets hide` pattern.
3. Definition of done: `sc stack outputs` does not print a secret-marked resource output in the clear unless the invoker explicitly opts in; a test proves the default invocation masks and the opt-in flag reveals.

## CRITICAL — this is the 3rd dispatch attempt. Both prior attempts (runs 32521357, fadc2560) died in the developer character to a guest OOM crashloop (`signal: killed` / `exit status 143`, repeated across fresh workspaces) with ZERO source commits and ZERO PRs opened, despite the Architect handoff signing off cleanly both times. This is a known, documented infra issue — see `docs/roadmap/items/api-repo-slices-oom-the-guest.md` in the `forge` repo for full detail. Two independent OOM mechanisms are in play: (a) a single `go build`/link can exceed 12GB RAM in the guest with zero swap, and (b) the guest is NOT recycled between resumes, so RAM accumulates across a long multi-resume turn even for cheap commands.

**Mandatory build discipline for the Developer character:**
- NEVER run `go build ./...`, `go test ./...`, or `make build`/`make build-all` (the link step alone needs ~13GB and WILL OOM a 12GB guest).
- ONLY use `make check` (capped `go vet ./...`, ~2.3GB) and narrowly-scoped `go test ./pkg/<touched-package>/...` for the specific package(s) you changed.
- Keep the turn SHORT — minimize resumes. If you find yourself on resume >5-10 without having committed, stop exploring and commit what you have; the guest's free RAM decays with resume count even for `make check`.
- If a build/test command appears to hang or use excessive memory, kill it immediately rather than waiting — do not retry the same command.
- This repo has `api/Makefile` and `api/CLAUDE.md` (merged via api#441) — read `api/CLAUDE.md` FIRST for the exact constraint and recognize `exit status 143` / `signal: killed` with empty stderr as a guest OOM, not a code problem.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All changes present and lint-clean. Now I'll produce the handoff.

---

# Roadmap Driver handoff — run cd4b63ec

**No code mutations in this turn.** This is a Roadmap Driver run — repo-roadmap bookkeeping + SDLC dispatches only; no service source was touched.

## 1. Commit + branch identity

No commit made directly by me — this run's edits are committed automatically by the workflow engine on `«branch-from-triggering-run-redacted»`.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +10/-6 | Removed 2 archived items' rows; updated 2 `api` items' rows to 🚧 IN PROGRESS with 2026-10-19 dispatch |
| docs/roadmap/archive/README.md | +2/-0 | Added `chat-refusal-masked-as-budget` and `sha-gate-treats-devops-as-developer` |
| docs/roadmap/items/aws-provider-key-in-deploy-log.md | +20/-4 | Reconciled 5th dispatch (run 29edcb53, failed, guest OOM, no PR) → `redispatch` |
| docs/roadmap/items/sc-fs-state-dir-not-gitignored.md | +20/-4 | Reconciled 1st dispatch (run 555f870a, failed, guest OOM, no PR) → `redispatch` |
| docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md | +17/-4 | Dispatched 3rd attempt → `in-progress` |
| docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md | +17/-4 | Dispatched 2nd attempt → `in-progress` |
| docs/roadmap/items/chat-refusal-masked-as-budget.md | +8/-1 | Archived: `archived: "2026-10-19"` + History line |
| docs/roadmap/items/sha-gate-treats-devops-as-developer.md | +5/-1 | Archived: `archived: "2026-10-19"` + History line |
| docs/roadmap/driver-log/2026-10-19.md | new file | Run notes |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile 🚧 IN PROGRESS slices; flip to ✅ SHIPPED if SDLC PR merged | PASS | Both `dispatched:`-set items (`aws-provider-key-in-deploy-log` run `29edcb53`, `sc-fs-state-dir-not-gitignored` run `555f870a`) checked via `get_run` + `list_pull_requests`/`list_repository_commits` on `simple-container-com/api` — both terminal `status: failed`, zero matching PR/commit → flipped to `redispatch`, not `shipped` (correct per evidence) |
| AC2 | Pick next un-done/un-in-progress slice | PASS | Pickable pool audited across whole README (REDISPATCH→NEXT→CODE-COMPLETE, `dispatch: auto` only) = exactly 4 rows, all `repo: api` |
| AC3 | Dispatch Main Forge SDLC | PARTIAL-PASS | 2 of 4 intended dispatches succeeded (`sc-stack-outputs-leaks-secrets-in-clear`, `sc-stack-outputs-empty-byo-vm-host-fields`); the other 2 (`aws-provider-key-in-deploy-log`, `sc-fs-state-dir-not-gitignored`) hit `trigger_workflow`'s per-run fan-out cap of 2 (`not_eligible: max follow-up fan-out (2) reached`) and remain `redispatch` for next run — recorded explicitly in History, not silently dropped |
| AC4 | Mark picked slice(s) IN PROGRESS | PASS | Both successfully-dispatched items flipped `redispatch`→`in-progress`, `dispatched: "2026-10-19"` set, index rows updated to 🚧 IN PROGRESS |
| AC5 | Archive pass (STEP 2) | PASS | 2 items qualified (shipped ≥2 days ago, no outstanding markers, untouched this run): `chat-refusal-masked-as-budget`, `sha-gate-treats-devops-as-developer` — archived |
| AC6 | Escalate stale/redispatch items (STEP 5b) | PASS | One Telegram notification sent covering both reconciled-to-redispatch items |
| AC7 | Lint clean | PASS | `bash docs/roadmap/lint.sh` → `408 items, 408 index rows, README.md 234 lines — 0 errors, 0 warnings` |

## 4. Tests run

```
$ bash docs/roadmap/lint.sh
408 items, 408 index rows, README.md 234 lines — 0 errors, 0 warnings
```

## 5. Notes for next run

The entire auto-pickable pool this run was `repo: api`, which has now failed 6 dispatch attempts across 3 items in 2 days with zero PRs — all guest-OOM crashloops, two independent mechanisms (single-process link OOM, and resume-accumulation OOM). Both dispatched this run carry explicit `make check`-only / short-turn guidance. `aws-provider-key-in-deploy-log` and `sc-fs-state-dir-not-gitignored` are queued for next run's dispatch (blocked this run only by the fan-out cap, not by any content issue). If the next reconcile still shows OOM deaths on any `api` slice despite the discipline, flip `repo: api` items to `dispatch: manual` and escalate the infra-side swapfile fix rather than continuing to redispatch.

**Verdict:** signoff
workflowTriggerParentRunId: cd4b63ec-8a93-4208-a064-20ca8ae5bf4a
workflowTriggerRootRunId: cd4b63ec-8a93-4208-a064-20ca8ae5bf4a