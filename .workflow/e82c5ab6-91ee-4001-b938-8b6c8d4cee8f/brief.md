[api-repo-branch-protection-blocks-sdlc-automerge] api repo's branch protection blocks every SDLC auto-merge, unlike every other dispatchable repo

## Background

The fix is fully merged + deployed: forge-conductor PR #583 (`RequiresPullRequestReview` in `github/client.go` + `autoMergeBlockedByReviewRequirement` gate in `orchestrator_pr.go`) and PR #597 (its typed-slice correction, commit `b72f8688`) are both on forge-conductor `main`, and `deploy-forge-conductor.yml` run `38993115393` deployed `b72f8688` successfully at `2026-10-17T01:59:15Z`. The fix is live on staging right now.

The live ruleset on `simple-container-com/api` (`GET /repos/simple-container-com/api/rules/branches/main`, ruleset id `9144757`) has been independently fetched multiple times across prior runs and is byte-identical to the test fixture pinned in forge-conductor commit `ea6b6bea96`. The unit/mutation-level proof is done.

**Root cause of why this has stalled for six prior dispatches, now fixed:** this roadmap item's own `repo:` front-matter said `forge-conductor` instead of `api`. The Roadmap Driver's dispatch table resolves `linked_repo_id` from the item's `repo:` field, so every prior dispatch (including the one that produced this very brief's predecessor) was routed to `forge-conductor`, not `api` — meaning no prior run could ever open the proof PR against `api` at all. This dispatch is the FIRST one correctly routed to `api` itself.

## The ONE remaining gap

Nobody has yet observed a REAL bot-opened PR against `simple-container-com/api` complete without an auto-merge escalation, post-fix. This dispatch, being linked directly to `api`, is positioned to finally supply that proof.

## Task for this dispatch

1. Confirm current state: `GET /repos/simple-container-com/api/rules/branches/main` still shows the review-requiring ruleset (expect unchanged — sanity check only, not new work).
2. Produce ONE live positive-path proof. Pick any genuinely trivial, safe, content-correct documentation/comment fix inside `simple-container-com/api` (e.g. a typo fix, a comment clarification, something with zero behavioral risk) and let the normal SDLC flow open a PR against it with `mergeStrategy: auto-merge`. Do NOT invent risky or large changes — the point is to observe the *merge/escalation* behavior, not to ship a feature.
3. Record what actually happens: does `maybeCreatePullRequest` open the PR as `mergeable_state: "blocked"`? Does the orchestrator correctly recognize `api`'s review-requiring ruleset post-fix and fall back to a normal (non-escalating) PR for human review — i.e. the run completes cleanly with "awaiting human review" rather than an auto-merge-escalation alarm?
4. Write the result (PR URL + observed `mergeable_state` + whether the run escalated or completed cleanly) into `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md`'s `## Status` and `## History` — this item's own file in the `forge` repo, not just your handoff. You have write access to `forge` via the GitHub App installation (ignore any `permission: none` collaborator-API reading — that's a known false negative for App tokens).
5. If the proof is clean (no spurious escalation), flip `status: code-complete` → `status: shipped` in that item's front-matter and mirror it into `docs/roadmap/README.md`'s index row (status cell + summary + next). If it reveals a remaining gap, leave it `code-complete` and record precisely what's missing.

This may be a verification-only turn with no `api`/`forge-conductor` source changes beyond the roadmap-doc update and whatever trivial fix you pick in `api` — that's fine; declare `**Empty-PR-OK:** yes` if applicable, per `sdlc-verification-only-runs-fail-substance-gate`.

## Definition of done (acceptance criteria, numbered)

AC1. `api`'s branch-protection ruleset confirmed unchanged (live GET).
AC2. A trivial, zero-risk PR opened against `simple-container-com/api` by this run.
AC3. Observed `mergeable_state` and merge/escalation outcome recorded with evidence (PR URL, API response).
AC4. The run completes WITHOUT a spurious auto-merge escalation (or, if it does escalate, record precisely why — distinguishing a genuine content defect from the known structural review-gate).
AC5. `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` and `docs/roadmap/README.md` updated to match the outcome; flip to `shipped` only if the proof is clean.
AC6. `docs/roadmap/lint.sh` passes (0 errors) after the roadmap edit.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All changes are consistent with the plan. Now let me produce the handoff document.

---

# Roadmap Driver Handoff — run a7aa5562

## Commit + branch identity

No code mutations — this is a pure roadmap-maintenance turn (STEP 0-7 of the Driver's own spec). My edits are to `docs/roadmap/` only, committed on this run's branch (`«branch-from-triggering-run-redacted»`) and will be auto-merged by the engine. I also merged a separate PR (`forge` #1603, squash commit `«sha-from-triggering-run-redacted»`) directly to `main` via the GitHub API as part of STEP 1 reconciliation — that commit is **not** mine, it's the prior SDLC run's DevOps fix; I pulled it into this branch via `git merge origin/main`.

## Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/roadmap/README.md` | +7/-5 | Inbox entry removed (intaken); new index row for `conductor-spa-render-blocking-google-fonts`; `api-repo-branch-protection-blocks-sdlc-automerge` row flipped to 🚧 IN PROGRESS; `aws-provider-key-in-deploy-log` row's Auto cell corrected `auto`→`**MANUAL**` to match its item file (was stale/inconsistent) |
| `docs/roadmap/items/conductor-spa-render-blocking-google-fonts.md` | +74/-0 (new file) | Filed from Inbox intake (STEP 0); grounded against `forge-conductor`'s `internal/ui/index.html`; later flipped `next`→`in-progress` on dispatch |
| `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +13/-5 | Front-matter `code-complete`→`in-progress`, `dispatched: "2026-10-17"`; `## History` entry for this run's dispatch |
| `docs/roadmap/driver-log/2026-10-17.md` | +61/-0 | Run note prepended (STEP 6) |

## Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | STEP 0: intake Inbox entries | PASS | One `### ` entry found (render-blocking fonts); filed as `items/conductor-spa-render-blocking-google-fonts.md` with grounded evidence from `forge-conductor`'s `internal/ui/index.html`; Inbox line deleted |
| AC2 | STEP 1: reconcile `🚧 IN PROGRESS` dispatches | PASS | Found one dispatched row (`api-repo-branch-protection-blocks-sdlc-automerge`); traced its prior run's handoffs, found + merged `forge` PR #1603 (squash `50a9d657`) which fixed the item's `repo:` metadata bug (was `forge-conductor`, should be `api` — the actual cause of 6 stalled dispatches) |
| AC3 | STEP 2: archive gate | PASS | Checked all 13 `shipped`+unarchived items; none qualify (stray status markers in body, or shipped <2 days ago); correctly declared no archive pass |
| AC4 | STEP 3: pick up to 4 slices | PASS | N=0 live dispatches; scanned index, found 3 auto-pickable candidates, excluded 1 (`aws-provider-key-in-deploy-log`, front-matter says `manual` despite a stale `auto` index cell — corrected the cell, did not pick it); picked the remaining 2 across different repos |
| AC5 | STEP 4: dispatch SDLC for picked slices | PASS | `trigger_workflow` → `forge-conductor` (`6a1d87679aa5469478145de4`) `{queued:true}` for fonts item; `trigger_workflow` → `api` (`6a1df1a2e639099d50d23c32`) `{queued:true}` for branch-protection item |
| AC6 | STEP 5: mark dispatched slices IN PROGRESS | PASS | Both item files flipped to `status: in-progress`, `dispatched: "2026-10-17"`; both README rows flipped to 🚧 IN PROGRESS |
| AC7 | STEP 5b: escalate stale items | PASS (N/A) | No items flipped to REDISPATCH this run — nothing to escalate |
| AC8 | STEP 6: log the run | PASS | Appended to `docs/roadmap/driver-log/2026-10-17.md`, newest-first |
| AC9 | STEP 7: lint passes | PASS | `bash docs/roadmap/lint.sh` → `402 items, 402 index rows, README.md 230 lines — 0 errors, 0 warnings` |

## Tests run

```
$ bash docs/roadmap/lint.sh
402 items, 402 index rows, README.md 230 lines — 0 errors, 0 warnings
```

No code tests applicable — this is a docs/roadmap-only turn (my role, PM/Driver, mutates no source).

## Summary of judgment calls

1. **Reconciliation surfaced a routing bug, not just a status flip.** The prior dispatch (`api-repo-branch-protection-blocks-sdlc-automerge`, dispatched by an earlier Driver run) had its SDLC run (`f7d3192d`) conclude the item's `repo:` front-matter was wrong — it said `forge-conductor` but should say `api`, which is why 6 prior dispatches against this slice could never open their proof PR (the Driver resolves `linked_repo_id` from `repo:`). That run opened `forge` PR #1603 with the fix, correctly held at `code-complete` (not over-claiming `shipped`). I reviewed and merged it (squash `50a9d657`), then pulled it into my branch before proceeding — this was necessary before I could re-dispatch the item correctly.
2. **Caught a stale index/item inconsistency.** `aws-provider-key-in-deploy-log`'s item file says `dispatch: manual` (correctly held after 4 consecutive developer-executor crashloops) but its README index cell had drifted to show `auto`. Per CONVENTIONS.md the front-matter governs; I did not pick it, and corrected the index cell to match.
3. **Filed one new item from Inbox** with grounded evidence (read the actual `internal/ui/index.html` source) rather than guessing — per STEP 0's instruction to verify before filing.

**Empty-PR-OK:** yes — this is the Roadmap Driver's standing role: a verification/reconciliation/dispatch turn that mutates only `docs/roadmap/` metadata and dispatches downstream SDLC work, never source code itself.

**Verdict:** signoff
workflowTriggerParentRunId: a7aa5562-0c30-49fb-968c-0da902295a18
workflowTriggerRootRunId: a7aa5562-0c30-49fb-968c-0da902295a18