[thinking] # PM Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` (6th dispatch)

## 0. Context restated

This is the 6th dispatch of this roadmap item (slice `api-repo-branch-protection-blocks-sdlc-automerge`). History so far:

- PR #583 (forge-conductor, merged, deployed) added the actual fix: `RequiresPullRequestReview` detection + `autoMergeBlockedByReviewRequirement` fallback gate.
- Negative control (no-ruleset repos still auto-merge): **proven** — PRs #584/#585 on forge-conductor.
- Positive-path proof (an `api`-targeted run reaching DevOps and observing the fallback fire): **never completed**. Most recent attempt, run `e82c5ab6` → PR #450, got through Developer+QA with a real fix, then died at an **unrelated Architect-role validator bug** (`verdictResolved=<unparseable>`, resumeCount 14) before DevOps ever ran. PR #450 is still open/draft/unmerged.

I independently reconfirmed live, just now (2026-10-18T00:37Z):

- `GET /repos/simple-container-com/api/rulesets/9144757` → `enforcement: active`, rule `pull_request` with `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `allowed_merge_methods: ["squash"]`, plus `required_status_checks` (7 named CI jobs) and `required_signatures`. `current_user_can_bypass: "never"`. **Unchanged** from prior dispatches — AC1 of the brief is pre-confirmed.
- PR #450 is still open (draft, title `[escalated] …`), diff is entirely `.workflow/…` metadata + a 2-line `docs/CONTRIBUTING.md` edit. It is NOT this run's artifact — leave it alone (operator can hand-merge or close later), do not touch it.
- `docs/CONTRIBUTING.md` on `main` **still contains the stale line** PR #450 was trying to fix: *"current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land."* — but the live ruleset already requires `required_approving_review_count: 2`. This is a genuine, still-unfixed doc inaccuracy, independent of PR #450 (which never merged). It is the ideal trivial payload for this run: real, truthful, zero behavioural risk, and happens to be topically on-point (it's literally about the review-count rule this item is about).

## 1. Requirements (restated from brief)

Produce a trivial, safe `api`-targeted SDLC change and carry it **all the way through DevOps**, to observe:

1. PR opens against `api`'s `main` with the review ruleset still active (2 reviews, code-owner review).
2. `autoMergeWithBackoff` attempts auto-merge, cannot satisfy the review requirement, and `autoMergeBlockedByReviewRequirement` fires cleanly — run completes as a normal actionable open PR, not an unexplained escalation.
3. If an orchestration-layer bug (not the fix itself) intervenes again, that must be recorded as a **distinct defect**, not conflated with "the fix is broken."

## 2. Scope decisions

| Item | Label | Rationale |
|---|---|---|
| Fix the stale "1 reviewer minimum" line in `docs/CONTRIBUTING.md` → reflect the real `required_approving_review_count: 2` | **MUST** | This IS the trivial payload. Doc-only, truthful, zero risk, directly on-topic. Developer should change just this sentence (and the review-required bullet nearby if it also mis-states the count) — nothing else. |
| Leave PR #450 untouched | **MUST** | It's a different run's open artifact; touching/closing it is an operator decision, not ours. Do not duplicate branches or interfere with it. |
| Carry the chain through to DevOps and record live evidence of the auto-merge attempt / fallback | **MUST** | This is the entire point of the dispatch — the thing 5 prior attempts failed to observe. |
| Distinguishing an orchestration bug from a fix defect if something breaks again | **MUST** | Brief explicitly requires this; don't let a validator crash get recorded as "the branch-protection fix is broken." |
| Update the roadmap item's `## Status` + `## History`, flip to `shipped` iff the live-dispatch clause is finally closed cleanly | **MUST** | Definition of done, explicit in brief. |
| Re-litigating which of the 4 candidate long-term directions (bypass actor, second ruleset, bot-exempt, etc.) to adopt | **CUT** | Already out of scope — PR #583 shipped direction #4 (teach the gate the repo is structurally never-auto-mergeable). Not this dispatch's job. |
| Any Go/behavioural code change | **CUT** | Brief explicitly says docs/comment-level is fine — the point is the merge-path behavior, not the diff content. Keep Developer's diff to the one doc file. |
| Retrying with `mergeStrategy` tuning, CI config changes, etc. | **CUT** | Out of scope; the fix under test lives entirely in forge-conductor's orchestrator, already deployed. This run only needs to *exercise* it. |

## 3. Acceptance criteria (for QA/DevOps to verify with live evidence)

| AC# | Criterion | How to verify |
|---|---|---|
| AC1 | Ruleset `9144757` still active with `required_approving_review_count: 2` at time of this run's merge attempt | `GET /repos/simple-container-com/api/rulesets/9144757` (or rules-for-branch endpoint), capture response + timestamp |
| AC2 | Developer's change is genuinely trivial (docs-only, the stale reviewer-count line in `docs/CONTRIBUTING.md`) and lands as a commit on this run's branch | commit SHA + diff showing only the doc line(s) changed |
| AC3 | PR opens against `api:main` from this run's branch | PR number + URL |
| AC4 | `autoMergeWithBackoff` attempts merge and cannot satisfy the review gate (0/2 reviews) | PR's `mergeable_state`/`reviewDecision` fields at merge-attempt time |
| AC5 | `autoMergeBlockedByReviewRequirement` fallback fires — run completes in a clean, non-alarming state (normal open PR, NOT an `[escalated]`-prefixed title, no validator-unparseable crash) | DevOps handoff quoting the orchestrator's own log/decision, PR title/state post-run |
| AC6 | If something else breaks the chain before DevOps, the handoff explicitly separates "orchestration-layer bug" from "the auto-merge fix is broken" | Whichever character hits the break names the exact failure mode and states which bucket it falls in |
| AC7 | `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` `## Status`/`## History` updated with this run's outcome; `status:` flips to `shipped` only if AC3–AC5 all hold cleanly, otherwise stays `in-progress`/`code-complete` with the specific blocker named | Diff of that file (on the `forge` repo, not this run's branch — note repo split below) |

## 4. Handoff guidance for Architect / Developer

- **Target repo/branch for the code change:** `simple-container-com/api`, this run's branch (`forge/workflow/.../run/2e40aa56-...`), base `main`.
- **Exact payload:** in `docs/CONTRIBUTING.md`, under "### Pull request", the bullet reads *"Code review is **required** — current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land."* — this is now false (the ruleset already requires 2 approving reviews + code-owner review, confirmed live above). Rewrite to state the true current requirement (2 approving reviews, code-owner review required) and drop the "will increase" framing since it's already in effect. Do not touch anything else in the file. This is a comment/doc-level change exactly as the brief calls for — no Go files, no behavior change, no test surface.
- **Do not** try to pre-empt or simulate the merge decision yourselves — that's DevOps/orchestrator's job once QA signs off. Your job is just to land the one accurate doc line and sign off cleanly so the chain actually reaches DevOps (unlike run `e82c5ab6`, which never did).
- **QA:** keep your own verdict JSON well-formed — the entire point of this dispatch is that the previous run died to an *Architect-role validator* producing unparseable verdict JSON, not a QA content issue. If you encounter the same validator misbehavior, call it out explicitly by name in your handoff (`condition_blocked(transition=qa-to-devops)` pattern) so it's unambiguous this is orchestration, not your signoff.
- **DevOps:** this is the crux turn. Capture: the PR number, the ruleset GET response at merge-attempt time, whatever auto-merge attempt/backoff evidence is observable, and whether `autoMergeBlockedByReviewRequirement` actually produced a clean non-escalated completion. Update the roadmap item yourself if you're the one who reaches furthest (whoever reaches furthest owns the roadmap update per the brief's DoD, note the roadmap lives in the `forge` repo, a different repo from the code change in `api` — two separate commits/PRs).
- **Repository note:** the code change is in `api`; the roadmap-status update is in `forge` (`docs/roadmap/items/...`). Don't conflate the two — commit to each repo separately, on this run's branch where applicable.

## 5. Files touched (this turn)

No code mutations in this turn — this is a PM planning turn.

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 6. Acceptance criteria verification (PM turn)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| Pre-check | Ruleset still active, `required_approving_review_count: 2` | PASS | Live `GET /repos/simple-container-com/api/rulesets/9144757` → 200, `required_approving_review_count: 2`, `require_code_owner_review: true`, `current_user_can_bypass: "never"` (captured above, request served 2026-10-18T00:37:54Z) |
| Pre-check | PR #450 (prior attempt) state confirmed, not to be disturbed | PASS | `list_pull_requests`/`read_pull_request_diff` on #450: open, draft, title `[escalated] …`, diff is `.workflow/` metadata + 2-line `docs/CONTRIBUTING.md` edit, unmerged |
| Pre-check | A real, trivial, truthful doc payload identified for Developer | PASS | `docs/CONTRIBUTING.md` on `main` still states "1 reviewer minimum; this will increase to 2" — contradicts live ruleset (already 2). Confirmed via `read_repository_file` |

## 7. Tests run

None — PM turn, no code mutated.

## 8. Verdict

Plan is complete, scope is tight (one doc line), ACs are explicit and checkable with live evidence, and handoff guidance names exact file/repo/branch targets for Architect/Developer/QA/DevOps. No blockers on my end.

**Verdict:** signoff