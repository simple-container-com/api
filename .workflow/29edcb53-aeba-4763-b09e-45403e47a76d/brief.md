[aws-provider-key-in-deploy-log] SC prints secret-marked provider credentials into the Actions log

⚠️ 5TH DISPATCH ATTEMPT — 4 prior attempts on this exact slice died in the Developer character from guest OOM kills (`signal: killed` / `exit status 143`), confirmed via host journal correlation (OOM 2-3s before each executor failure). This repo (`simple-container-com/api`) runs in a 12GB swapless microVM. THE FIX FOR THIS IS NOW MERGED: api#441 (commit `6fd77975`+`48be59c3`) added `api/Makefile` and `api/CLAUDE.md` — READ `api/CLAUDE.md` FIRST, it documents this exact constraint. `make check` (`go vet ./...`, measured 2.3GB peak) is the ONLY full-repo command proven to fit. `make build`/`make build-all` link `cmd/sc` and peak at 12.9-14.2GB — THESE WILL OOM, do not run them, do not run bare `go build ./...` or `go test ./...` either.

GOAL: a failed isolated deploy printed a live AWS access key AND its secret in plaintext, 4x in one log. The leak is generic — any secret-marked property of any resource/provider on any cloud can print this way via `pkg/clouds/pulumi/events.go:75` (`procDiagnosticEvent`), which forwards the raw gRPC diagnostic message at Info level.

SCOPE IS NARROW — read this before touching events.go: `pkg/api/logger/logger.go:69` already wraps every `Info` call in `Redact(...)`, and `pkg/clouds/pulumi/provisioner.go:26` confirms `pulumi.logger` IS that `logger.Logger` — so `procDiagnosticEvent` is ALREADY behind the redaction chokepoint (shipped in api#429, commit `dc6a461f`). `events.go` needs NO changes. The actual gap is in `pkg/api/logger/redact.go:46`: `Redact` applies four patterns (privateKeyBlock, privateKeyTail, privateKeyID, base64Run-that-decodes-to-a-private-key) and has ZERO pattern for an AWS access-key id (`AKIA[0-9A-Z]{16}` shape) or a 40-char AWS secret access key. Add those two patterns, following the existing test precedent in `pkg/clouds/pulumi/preview_redact_test.go` and extending `redact_test.go`.

DEFINITION OF DONE (gated ONLY on `make check` + `go test ./pkg/api/logger/...` — never a broader build/test):
1. `redact.go` matches and redacts AWS access-key ids (`AKIA...` 20-char) and AWS secret access keys (40-char base64-ish) wherever they appear in a log message, matching the existing pattern style (regex + replacement).
2. `redact_test.go` has new table cases proving both patterns redact correctly and don't false-positive on adjacent text.
3. Do NOT touch `events.go`, `provisioner.go`, or the key-rotation/revocation logic (S1/S2 in the item file are already handled/operator-owned) — this slice is `redact.go` only.

slice_id: aws-provider-key-in-deploy-log

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