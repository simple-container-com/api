[thinking] # Architect Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` (6th dispatch)

## 0. Role scope for this turn

This is a design/review turn, not an implementation turn. PM's plan is sound and tightly scoped; my job is to (a) independently re-verify the live facts the plan depends on, (b) pin down the *exact* diff so Developer has zero ambiguity, and (c) flag any structural risk before code gets written. I did not mutate any files — the one-line doc fix belongs to Developer, per PM's own handoff guidance.

## 1. Commit + branch identity

No code mutations in this turn.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 3. Independent verification (live, this turn)

- `GET /repos/simple-container-com/api/rulesets/9144757` → 200, re-confirmed just now (2026-10-18T00:39:34Z, `Etag: W/"e4be21..."`):
  - `enforcement: "active"`
  - `pull_request` rule: `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `required_review_thread_resolution: true`, `allowed_merge_methods: ["squash"]`
  - plus `required_status_checks` (7 named CI contexts incl. `Run tests`), `required_signatures`, `non_fast_forward`, `deletion`
  - `current_user_can_bypass: "never"`, `bypass_actors: []`
  - Unchanged since PM's check — AC1 of the brief is solid.
- Read `docs/CONTRIBUTING.md` on `main` directly (sha `05dd517...`). Confirmed the exact stale line PM flagged, verbatim:
  > `- Code review is **required** — current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land.`
  
  This is the only sentence in the file that references the review count. No other mention exists elsewhere in the doc.
- Cross-checked against `CLAUDE.md` (root, sha `626ed92...`), which independently states: *"Branch protection also requires human review — nothing self-merges here."* — consistent with the live ruleset and with this item's whole premise. No contradiction to flag.
- `list_repository_tree` on repo root confirms `docs/CONTRIBUTING.md` is the intended, existing file (not a path Developer needs to create).

## 4. Design decision: the diff, pinned exactly

**File:** `docs/CONTRIBUTING.md`, under `### Pull request`.

**Replace this line:**
```
- Code review is **required** — current branch protection enforces 1
  reviewer minimum; this will increase to 2 once admin-UI gates land.
```

**With:**
```
- Code review is **required** — branch protection enforces 2 approving
  reviews plus code-owner review before merge; nothing self-merges here.
```

Rationale for the exact wording:
- Drops the "will increase" future-tense framing since it's already true today (confirmed live above) — leaving stale future-tense language in would be a *new* inaccuracy in the other direction.
- "nothing self-merges here" deliberately echoes `CLAUDE.md`'s own phrasing so the two docs stay consistent instead of drifting into two different descriptions of the same rule.
- No other line in the file references review count, so this is a single, self-contained, zero-blast-radius edit — exactly what the brief calls for ("docs/comment-level is fine — the point is observing the merge-path behavior, not the diff content").

**Scope guard for Developer:** touch only this one list item. Do not touch the surrounding TL;DR numbered list (item 3, about signed commits, is unrelated and must stay as-is), do not touch `CODEOWNERS`, do not touch any `.go` file. A single-hunk diff keeps QA's job trivial and keeps the PR itself an honest negative-risk artifact for the merge-path experiment.

## 5. Risk check (architect sign-off reasoning)

- **Structural risk:** none — doc-only, no CI surface, no runtime behavior.
- **Does this change interact with the required status checks?** No — none of the 7 required checks (build/test matrix) touch `docs/`. The PR will still need to pass them per the ruleset, but a docs-only change has no reason to fail them.
- **Signed-commit requirement:** Developer must sign the commit (`git commit -S -s` or SSH signing, per this same `CONTRIBUTING.md`/`CLAUDE.md`) — the ruleset carries `required_signatures`. If Developer's commit lands unsigned, the PR will be blocked on that gate too, which would muddy the observation (we want the *review*-gate fallback to be the isolated, observed cause, not a signature failure). Flagging this explicitly so Developer doesn't skip it.
- **Does not pre-empt or simulate DevOps's merge-attempt work** — left entirely to DevOps/orchestrator per PM's plan.
- **PR #450 interference:** confirmed via repo read this is a different branch/PR; nothing in this design touches it.

## 6. Acceptance criteria verification (architect-level, design review only)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Ruleset `9144757` active, `required_approving_review_count: 2` | PASS | Live GET above, 200, `required_approving_review_count: 2`, `current_user_can_bypass: "never"` (2026-10-18T00:39:34Z) |
| AC2 (design) | Exact trivial, truthful, single-hunk payload identified | PASS | `docs/CONTRIBUTING.md` sha `05dd517...` read in full; stale line located and exact replacement text specified above, scoped to one list item |
| AC3–AC6 | PR open / auto-merge attempt / fallback fires / orchestration-bug distinction | N/A for this turn | Owned by Developer (commit) → QA (verify diff scope) → DevOps (observe merge-path); not verifiable from a design-only turn |
| AC7 | Roadmap item status update | N/A for this turn | Owned by whichever downstream character reaches furthest, per brief DoD — not yet reached |

## 7. Tests run

None — no code mutated this turn. `go vet`/`make check` not applicable to a markdown-only change; Developer should still run `make check` after committing per `CLAUDE.md`'s guidance as a sanity pass (cheap, 2.3GB/~3min) even though this diff can't plausibly break Go compilation.

## 8. Handoff guidance for Developer

- Branch: this run's branch, base `main`, repo `simple-container-com/api`.
- Make exactly the one-line edit specified in §4 above to `docs/CONTRIBUTING.md`. Nothing else.
- Sign the commit (`-S -s`) — ruleset requires signatures; an unsigned commit would confound the observation this run exists to make.
- Commit message: Conventional Commits form, e.g. `docs(contributing): correct stale review-count description to match live branch protection (2 approvals)`.
- Open PR against `api:main`. Hand to QA to confirm diff is single-hunk/doc-only, then to DevOps to observe and record the merge-path behavior (PR state, ruleset re-check, whether `autoMergeBlockedByReviewRequirement` fires cleanly vs. an orchestration-layer break) — per PM's AC3–AC7.
- Roadmap update (`forge` repo, `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md`) is owned by whichever character reaches furthest in this chain — do not update it prematurely from a Developer turn before the merge-path outcome is actually observed.

**Verdict:** signoff