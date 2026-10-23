[api-repo-branch-protection-blocks-sdlc-automerge] api repo's branch protection blocks every SDLC auto-merge, unlike every other dispatchable repo

## Background

`simple-container-com/api` carries an active branch-protection ruleset (`9144757`, "Protect main branch") requiring 2 approving reviews + code-owner review before any merge — unlike `forge-conductor` and most other dispatchable repos, which have no such PR-review ruleset. A completed SDLC run against `api` therefore opens a mergeable-but-blocked PR, and the merge-evidence gate escalates it as if the content were defective, when the real cause is structural (zero of the required reviews, not a bad diff).

forge-conductor PR #583 (merged `f6e9da7758239ca7fb476c4212f58b2e1c0dffa4`, deployed) already added the fix: `RequiresPullRequestReview` (detects the ruleset) + an `autoMergeBlockedByReviewRequirement` fallback gate in the orchestrator, so a run against a review-gated repo should end in a PR a human can act on with one click, not an alarming escalation indistinguishable from a real content failure.

**What's still unproven, after 4 prior dispatches of this exact item:**
- The negative control passed (no-ruleset repos like forge-conductor still auto-merge cleanly post-fix — PRs #584/#585, 3-4s each).
- But the **positive-path proof against `api` itself has never completed**. The most recent attempt (run `e82c5ab6`, PR #450 on `api`) got all the way through Developer + QA signoff with a real 2-line fix, then the run **escalated upstream of the merge decision** on an unrelated Architect-role validator bug (`condition_blocked(transition=qa-to-devops): verdictResolved=<unparseable>`, resumeCount 14) — the DevOps character that would exercise `autoMergeBlockedByReviewRequirement` never ran. PR #450 is still open (draft, unmerged) as of this dispatch.

## Task

Produce a small, safe, genuinely trivial change against `simple-container-com/api` (docs/comment-level is fine — the point is observing the merge-path behavior, not the diff content) and carry it all the way through DevOps. The specific thing this dispatch needs to observe and record, with live evidence (not inference):

1. The PR opens against `api`'s `main` with the review-requirement ruleset active (reconfirm via `GET /repos/simple-container-com/api/rulesets/9144757` or the rules-for-branch endpoint — it should still show `required_approving_review_count: 2`).
2. `autoMergeWithBackoff` attempts auto-merge, cannot satisfy the review requirement, and the `autoMergeBlockedByReviewRequirement` fallback (added in PR #583) actually fires — i.e. the run completes in a state the operator can act on (a normal open PR awaiting review) rather than escalating as an unexplained failure.
3. If the run instead dies to an orchestration-layer issue again (e.g. a validator producing non-JSON output, an infra crash) before reaching DevOps, record that explicitly as a DISTINCT defect from this item's actual subject — do not let it masquerade as a disproof of the `autoMergeBlockedByReviewRequirement` fix, which has already passed its negative control.

## Definition of done

- A live `api`-targeted SDLC dispatch reaches DevOps and demonstrates either: (a) a clean fallback to a normal, non-alarming open PR when the review gate blocks auto-merge, or (b) if something else interrupts the run, a clear diagnosis that distinguishes "orchestration-layer bug" from "the branch-protection fix itself is broken."
- Update `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md`'s `## Status` + `## History` with the outcome and evidence (PR number, ruleset state, whether DevOps ran, whatever character reaches furthest). If the live-dispatch clause is finally closed cleanly, flip `status: code-complete` → `status: shipped`. If it's blocked again by the same or a new orchestration bug, say so plainly rather than re-asserting "shipped" or "code-complete" without new evidence.
- You have write access to the `forge` repo (the roadmap) via the GitHub App installation for this org — ignore any `permission: none` collaborator-API false negative, it's a known false reading for App tokens.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All as expected. Now let me produce the handoff.

## Handoff — Forge Roadmap Driver (run 6f37b4df)

### 1. Commit + branch identity
No code mutations — this is a roadmap-only Driver turn. All edits are to `docs/roadmap/` files on this run's branch; they will be committed by the engine. I additionally merged an upstream PR directly via the GitHub API (not a commit on this branch): `simple-container-com/forge` PR #1613, merge commit `«sha-from-triggering-run-redacted»`, which flipped `conductor-spa-render-blocking-google-fonts` to `code-complete` (opened by the prior SDLC run's devops character — I merged rather than duplicate the edit).

### 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +4/-3 | Reconciled 1 shipped item, intook 2 Inbox entries into their own items + index rows, marked 2 newly-dispatched items IN PROGRESS |
| docs/roadmap/items/conductor-spa-render-blocking-google-fonts.md | +11/-6 | `code-complete` → `shipped`, live-verification evidence appended to `## History` |
| docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md | +12/-9 | `code-complete` → `in-progress` (5th dispatch), `## History` line added |
| docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md | +41 (new) | Filed from README Inbox, then dispatched → `in-progress` |
| docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md | +35 (new) | Filed from README Inbox; left `next` (fan-out cap reached) |
| docs/roadmap/driver-log/2026-10-18.md | +48 (new) | This run's log entry |

### 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | STEP 0 — intake stray Inbox entries | PASS | Filed both `### ` k3s-smoke defects into `sc-stack-outputs-leaks-secrets-in-clear.md` and `sc-stack-outputs-empty-byo-vm-host-fields.md`, cleared the Inbox |
| AC2 | STEP 1 — reconcile IN PROGRESS dispatches | PASS | Merged `forge` PR #1613, then live-verified `app.simple-forge.com` itself (0 Google Fonts refs, `font/woff2` content-type, valid wOF2 bytes) — promoted `conductor-spa-render-blocking-google-fonts` all the way to `shipped`, not just `code-complete` |
| AC3 | STEP 2 — archive gate | PASS | Correctly found no candidates: only 2 shipped items have zero outstanding markers, both 1 day short of the 2-day gate |
| AC4 | STEP 3 — pick up to 4 slices | PASS | Picked all 3 auto-pickable rows in the index (1 pre-existing + 2 newly intook) |
| AC5 | STEP 4 — dispatch | PASS (2/3) | `api-repo-branch-protection-blocks-sdlc-automerge` and `sc-stack-outputs-leaks-secrets-in-clear` both queued successfully; third (`sc-stack-outputs-empty-byo-vm-host-fields`) rejected by the engine's own 2-per-run follow-up fan-out cap — left `next` for next run, per STEP 4's "one error must never stop the other dispatches" rule |
| AC6 | STEP 5 — mark dispatched slices IN PROGRESS | PASS | Both successful dispatches flipped `status`/`dispatched`/`updated`/`## History` and their README rows |
| AC7 | STEP 6 — log the run | PASS | `docs/roadmap/driver-log/2026-10-18.md` created, newest-first, lists intake/reconcile/archive/pick/dispatch with evidence |
| AC8 | STEP 7 — lint clean | PASS | `bash docs/roadmap/lint.sh` → `406 items, 406 index rows, README.md 234 lines — 0 errors, 0 warnings` |

### 4. Tests run
```
$ bash docs/roadmap/lint.sh
406 items, 406 index rows, README.md 234 lines — 0 errors, 0 warnings
```

### 5. Verdict line

**Empty-PR-OK:** no — this run makes real roadmap-state changes (item files, README index, a merged upstream PR) that must land, not pure verification.

Summary: reconciled `conductor-spa-render-blocking-google-fonts` all the way to `shipped` (merged its pending roadmap PR, then independently live-verified the font fix on prod). Intook 2 stray Inbox defects into their own items. Dispatched 2 of 3 eligible auto slices (`api-repo-branch-protection-blocks-sdlc-automerge`'s 5th attempt at proving its auto-merge-fallback fix live; `sc-stack-outputs-leaks-secrets-in-clear`'s secret-masking fix) — the third hit the engine's per-run fan-out cap and stays queued for next time. Lint clean.

**Verdict:** signoff
workflowTriggerParentRunId: 6f37b4df-8e09-4a74-aae5-7d88bf7de629
workflowTriggerRootRunId: 6f37b4df-8e09-4a74-aae5-7d88bf7de629