[sc-stack-outputs-leaks-secrets-in-clear] sc stack outputs prints secret-marked outputs in the clear, with no flag to mask them

## Background

Live-caught during a 2026-10-18 k3s smoke of `simple-container-com/api`'s `sc` CLI: `sc stack outputs -s <stack>` prints every output value in the clear, including outputs that are secret-marked by the resource. In the smoke, a full cluster-admin kubeconfig and a k3s join token were both printed unmasked to the terminal output. There is currently no flag to mask or redact secret-marked values once they're shown — `--show-secrets` (if it exists) controls whether secret outputs are displayed at all, but once displayed they are always plaintext.

This matters because a kubeconfig with cluster-admin and a cluster join token are both high-value credentials — once printed to a terminal or CI log, they're effectively leaked (logs are often retained, screen-shared, or pasted into chat/tickets).

## Task

1. Find where `sc stack outputs` renders resource outputs (likely in the `api` repo's CLI/stack-outputs command implementation) and locate how it currently decides whether/how to show a secret-marked output.
2. Add masking: by default, a secret-marked output should render as a fixed placeholder (e.g. `<secret:redacted>`) rather than its plaintext value. Add an explicit opt-in (either reuse an existing `--show-secrets`-style flag if the display-vs-mask semantics can be cleanly separated, or add a new flag e.g. `--reveal-secrets`) that reveals the plaintext value when the invoker explicitly asks for it.
3. Mirror the "sensitive value, handle with care" pattern already used elsewhere in the `sc` CLI (e.g. `sc secrets reveal` / `sc secrets hide`) for consistency — same spirit, applied to resource outputs instead of the secrets registry.
4. Add or update tests covering: a stack output NOT marked secret is unaffected; a stack output marked secret is masked by default; the explicit reveal flag/option un-masks it.

## Definition of done

- `sc stack outputs` does not print a secret-marked resource output in the clear unless the invoker explicitly opts in.
- A test (unit or a documented manual smoke script) demonstrates: default invocation shows a masked/redacted value for a secret-marked output; the opt-in path reveals the real value.
- Record the outcome (PR number, test evidence) in `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`'s `## Status` + `## History` in the `forge` repo — you have write access to `forge` via the GitHub App installation for this org (ignore any `permission: none` collaborator-API false negative, it's a known false reading for App tokens). If the fix is clean and tested, flip `status: next` → `status: code-complete`; only flip to `shipped` after a live/deployed verification, per the repo's own CLAUDE.md definition of done.

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