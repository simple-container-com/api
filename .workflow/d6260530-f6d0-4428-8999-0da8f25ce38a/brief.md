[api-release-rerun-fails-existing-github-release] api: re-running a release run always fails at Create GitHub Release (tag already exists)

## Goal
`scripts/create-github-release.sh:110` calls `gh release create "${VERSION}" …` unconditionally. The script is `set -euo pipefail`, so a re-run of a release run that already created the GitHub Release for that CalVer tag dies with `a release with the same tag name already exists: <VERSION>`. Re-running failed jobs is the normal recovery path for this pipeline (image legs hit transient CDN/registry flakes routinely) — so every such recovery now ends in a red release even though every artefact built and published correctly.

## Scope
- Make `scripts/create-github-release.sh` idempotent: detect an existing release for the tag (e.g. `gh release view "${VERSION}" >/dev/null 2>&1`) and branch to `gh release upload --clobber` instead of `gh release create` when it already exists.
- Do NOT weaken the unsigned-release refusal: the script currently hard-fails when the dist bundle has zero `.sigstore.json` sidecars (around line 62). That guard must stay intact on a re-run — a re-run must still refuse to publish an unsigned release.
- Preview/RC tags must keep short-circuiting as they do today (around line 45) — don't touch that branch.

## Definition of done
1. A re-run of a release run whose GitHub Release already exists finishes green.
2. The existing assets are refreshed (clobbered) rather than the run dying on duplicate-tag creation.
3. The unsigned-release refusal is unchanged and still fires when the dist bundle lacks signatures.
4. Preview/RC tag handling is unchanged.
5. Add/update a test or a clear manual verification procedure for both branches (fresh tag → create; existing tag → upload --clobber).

## Context
Found 2026-10-21 live, while re-running the `github-actions` docker leg of api run 39571929882 to finish the live verification of the `api-release-binary-roundtrip` roadmap item (just shipped). That re-run's image build, `welder run tag-release`, `publish sc tool` and `docs--prod` deploy all went green, then it died on this exact script. Full repro and root cause are documented in `docs/roadmap/items/api-release-rerun-fails-existing-github-release.md` in the `forge` umbrella repo if more context is needed.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
Everything's in order. Here's the handoff.

---

# Roadmap Driver run handoff

## 1. Commit + branch identity
No separate commit SHA to cite here — these are roadmap-doc edits on this run's own branch `«branch-from-triggering-run-redacted»`, which auto-merges on completion per my role instructions.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/roadmap/README.md` | +9/-6 | STEP 0: added row + deleted Inbox entry for `meeting-agent-presentation-mode-fragment-gate`. STEP 2: deleted 2 archived rows. STEP 5: flipped 2 rows to `🚧 IN PROGRESS`. |
| `docs/roadmap/items/meeting-agent-presentation-mode-fragment-gate.md` | new file | Filed from the stray Inbox entry; `status: code-complete`, `dispatch: manual` (operator flag-flip + live call remains). |
| `docs/roadmap/items/escalated-run-strands-mergeable-code.md` | +8/-6 | Fixed two stale phase rows (Slice 2 code had shipped; follow-up item already shipped+archived), set `archived: "2026-10-21"`. |
| `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +6/-4 | Fixed stale "Fix" phase row (proven by the row below it), set `archived: "2026-10-21"`. |
| `docs/roadmap/archive/README.md` | +2/-0 | Added archive rows for the two items above. |
| `docs/roadmap/items/api-release-rerun-fails-existing-github-release.md` | +6/-4 | `status: next` → `in-progress`, `dispatched: "2026-10-21"`, History line for SDLC dispatch. |
| `docs/roadmap/items/baas-llm-log-leaks-program-secrets.md` | +6/-4 | `status: next` → `in-progress`, `dispatched: "2026-10-21"`, History line for SDLC dispatch. |
| `docs/roadmap/driver-log/2026-10-21.md` | +51/-0 | Appended this run's note (newest-first, above the earlier run's entry from today). |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | STEP 0: intake stray README entries | PASS | One `### ` Inbox entry found, filed as `items/meeting-agent-presentation-mode-fragment-gate.md` with full front-matter, body moved, index row added, Inbox entry deleted |
| AC2 | STEP 1: reconcile `🚧 IN PROGRESS` dispatches, flip to shipped/redispatch as evidenced | PASS | All 25 IN-PROGRESS rows checked; all have `dispatched: ""` (part-built milestones) — nothing to reconcile, confirmed via grep across every item file |
| AC3 | STEP 2: archive qualifying shipped items | PASS | 2 of 14 SHIPPED items passed the ≥2-day + no-open-marker gate (`escalated-run-strands-mergeable-code`, `api-repo-branch-protection-blocks-sdlc-automerge`); both archived with stale rows corrected first per CONVENTIONS |
| AC4 | STEP 3: pick up to 4 pickable+auto slices | PASS | Only 2 existed in the entire roadmap (`api-release-rerun-fails-existing-github-release`, `baas-llm-log-leaks-program-secrets`); both free of live runs (`list_runs sliceId=...` confirmed) |
| AC5 | STEP 4: dispatch Main Forge SDLC for each picked slice | PASS | Both `trigger_workflow` calls returned `queued:true` against the correct `linked_repo_id` (api `6a1df1a2e639099d50d23c32`, forge-baas `6a54de7f257a95aea5d316d2`) |
| AC6 | STEP 5: mark each dispatched slice IN PROGRESS | PASS | Both item files + index rows flipped to `in-progress` / `🚧 IN PROGRESS` with `dispatched: "2026-10-21"` |
| AC7 | STEP 5b: escalate stale items | PASS (n/a) | Nothing flipped to `redispatch` this run — no escalation needed |
| AC8 | STEP 6: log the run | PASS | Appended detailed note to `docs/roadmap/driver-log/2026-10-21.md`, above the prior run's entry from today |
| AC9 | STEP 7: lint passes | PASS | `bash docs/roadmap/lint.sh` → `416 items, 416 index rows, README.md 236 lines — 0 errors, 0 warnings` |

## 4. Tests run

```
$ bash docs/roadmap/lint.sh
416 items, 416 index rows, README.md 236 lines — 0 errors, 0 warnings
```

## 5. Dispatched this run
- `api-release-rerun-fails-existing-github-release` → Main Forge SDLC on `api`
- `baas-llm-log-leaks-program-secrets` → Main Forge SDLC on `forge-baas`

**Verdict:** signoff
workflowTriggerParentRunId: 5ddc13f7-4cc2-45c1-b0a9-5ad5535ef4ee
workflowTriggerRootRunId: 5ddc13f7-4cc2-45c1-b0a9-5ad5535ef4ee