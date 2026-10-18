[sc-fs-state-dir-not-gitignored] Nothing gitignores the fs state directory, so the default happy path invites committing the checkpoint

⚠️ CRITICAL BUILD CONSTRAINT — read before writing any code. This repo (`simple-container-com/api`) runs in a 12GB swapless microVM. `go build ./...` / `go test ./...` across the whole module WILL be OOM-killed (signal: killed / exit status 143) — this has happened repeatedly (see docs/roadmap/items/api-repo-slices-oom-the-guest.md in the forge umbrella repo for full detail, and api/CLAUDE.md + api/Makefile in THIS repo, merged 2026-10-17). Use `make check` (capped `go vet ./...`, ~2.3GB, safe) as your primary gate. If you need to run a scoped package test, run it narrowly: `go test ./pkg/provisioner/... ./pkg/git/...` (or whichever specific packages you touch) — never the bare `./...` wildcard, never `make build`/`make build-all` (these link `cmd/sc` and need ~13GB, which does not fit). If a command appears to hang or dies with no stderr, stop — do not retry with a bigger invocation.

GOAL: `state.type: fs` takes a plain `path:`. The natural configuration (e.g. `.sc/state` inside the checkout) is never added to `.gitignore`, so `sc init` + a `type: fs` provision leaves the checkpoint (deploy private key, full env file with every `${secret:...}` rendered, resource graph) offered for staging by `git status`, and `git add -A` takes it.

SCOPE: `pkg/clouds/pulumi/fs_state_store.go` (registered in `pkg/clouds/pulumi/init.go`) already has an init hook that `MkdirAll`s the path at mode 0700 before anything is written there — that is the right place to add the gitignore write. `git.Repo.AddFileToIgnore` already exists and is already called from two other places (`pkg/provisioner/common.go:248`, `pkg/api/secrets/management.go:82`) — follow that existing pattern, don't invent a new one.

DEFINITION OF DONE:
1. A `type: fs` provision whose `path` resolves inside the repo root leaves that path listed in `.gitignore`, written by SC, without the operator being told to do it by docs.
2. A path outside the repo root is NOT added to `.gitignore` (it cannot be committed and doesn't belong there).
3. Re-provision does not duplicate the `.gitignore` entry (assert `AddFileToIgnore`'s existing dedup behavior, don't re-implement it).
4. Remove the hand-written workaround instruction in `docs/docs/examples/vm-compose/byo-host/server.yaml` (added in api#455, commit `a845fd71`) once the real fix lands — it becomes stale advice.
5. Worth a quick look (not blocking): whether other DIY-backend-on-disk state paths have the same gap, and whether Pulumi's `.pulumi/history/`/`.pulumi/backups/` siblings need naming explicitly or are covered by ignoring the parent directory.

Tests: add/extend a unit test for the fs_state_store init hook covering in-repo vs out-of-repo paths and idempotent re-provision. Gate on `make check` + the scoped package test only — do not run a full build.

slice_id: sc-fs-state-dir-not-gitignored

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All changes are in place. Now producing the handoff.

---

# Roadmap Driver handoff — run e0cc2c61

No code mutations outside roadmap bookkeeping in this turn (I am the Roadmap Driver; my job is index/item-file maintenance + SDLC dispatch, not feature code).

## 1. Commit + branch identity

Edits committed to this run's branch (`«branch-from-triggering-run-redacted»`) by the auto-commit mechanism after this turn. No manual `git push` performed.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +4/-4 | Fixed one stale Auto-column drift (`aws-provider-key-in-deploy-log` MANUAL→auto); flipped 2 rows IN PROGRESS→REDISPATCH (reconcile), then 2 rows back to IN PROGRESS (new dispatches) |
| docs/roadmap/items/aws-provider-key-in-deploy-log.md | +7/-6 | `status: redispatch`→`in-progress`, `dispatched: "2026-10-18"`, History line for 5th dispatch |
| docs/roadmap/items/sc-fs-state-dir-not-gitignored.md | +6/-5 | `status: next`→`in-progress`, `dispatched: "2026-10-18"`, History line for dispatch |
| docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md | +9/-6 | Reconciled failed run 354c5876 → `redispatch`; left undispatched (fan-out cap) |
| docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md | +9/-6 | Reconciled failed run fadc2560 (2nd crashloop) → `redispatch`; left undispatched (fan-out cap) |
| docs/roadmap/driver-log/2026-10-18.md | +72/-1 | Prepended this run's log entry (newest-first) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile IN PROGRESS slices; flip to shipped if SDLC PR merged | PASS | Checked both `dispatched:`-set items (`sc-stack-outputs-leaks-secrets-in-clear` run fadc2560, `sc-stack-outputs-empty-byo-vm-host-fields` run 354c5876) against full `api` PR list (open+merged, 30 most recent) via `list_pull_requests` — neither referenced; both runs `status: failed`, Developer character died in a `signal: killed`/`exit 143` crashloop with zero commits. Flipped both to `redispatch` per CONVENTIONS.md STEP 1.4 |
| AC2 | Pick next un-done/un-in-progress slice | PASS | Audited whole README (197 rows, via subagent) for `dispatch: auto` pickable rows — found a drift (aws-provider-key-in-deploy-log), fixed it, then picked all 4 auto-pickable `⬜`/`🟡` rows in the entire index: `sc-stack-outputs-leaks-secrets-in-clear`, `sc-stack-outputs-empty-byo-vm-host-fields`, `sc-fs-state-dir-not-gitignored`, `aws-provider-key-in-deploy-log` |
| AC3 | Dispatch Main Forge SDLC | PARTIAL | 4 `trigger_workflow` calls attempted; 2 succeeded (`sc-fs-state-dir-not-gitignored`, `aws-provider-key-in-deploy-log` → both `{queued:true}`); 2 rejected by engine's own per-run fan-out cap (`not_eligible: max follow-up fan-out (2) reached`) — this is an engine-level limit independent of the roadmap's 4-slice-per-run policy, documented in the driver log and left for next run |
| AC4 | Mark picked slice(s) IN PROGRESS | PASS | Both successfully-dispatched slices flipped to `status: in-progress`, `dispatched: "2026-10-18"`, `updated: 2026-10-18`, History line added, README row → 🚧 IN PROGRESS. The 2 rejected slices correctly left as `⬜ REDISPATCH` (not marked in-progress, since no dispatch actually fired) |

## 4. Tests run

`bash docs/roadmap/lint.sh` → `408 items, 408 index rows, README.md 236 lines — 0 errors, 0 warnings`

## 5. Notes for next run / downstream

- Both `sc-stack-outputs-*` items are back in the auto-pickable pool as `⬜ REDISPATCH` — 2 and 1 crashloop deaths respectively today, same `signal: killed`/`exit 143` shape as the already-documented `api-repo-slices-oom-the-guest` class. Escalated to operator via Telegram this run.
- `aws-provider-key-in-deploy-log`'s README/item-file dispatch drift (MANUAL vs auto) is fixed; worth an operator spot-check of whether other items have similar silent drift in the future, since this one sat wrong for a full day undetected.
- Fan-out cap (2 trigger_workflow calls per Driver run, enforced by the engine) is tighter than the roadmap's stated 4-slice pick cap — worth a design note if this keeps costing picks, since it already bit two consecutive runs today.

**Empty-PR-OK:** no — this run did mutate real roadmap files (README + 4 item files + driver log), not just `.workflow/` metadata.

**Verdict:** signoff
workflowTriggerParentRunId: e0cc2c61-49c3-406c-afb6-1fd6f61c46a3
workflowTriggerRootRunId: e0cc2c61-49c3-406c-afb6-1fd6f61c46a3