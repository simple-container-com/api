[thinking] # PM Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` — Live Positive-Path Proof

## 0. Restated requirement (ruthlessly scoped)

This dispatch is **verification-only** with a **minimal, deliberately trivial code change** as the vehicle. The fix for the underlying bug (`RequiresPullRequestReview` + `autoMergeBlockedByReviewRequirement`) is already merged + deployed to forge-conductor (PR #583/#597, deploy `38993115393`, live since `2026-10-17T01:59:15Z`). Nothing here re-touches that logic. The ONLY open question: when a real bot PR lands against `simple-container-com/api` post-fix, does the orchestrator correctly fall back to a normal human-review PR instead of escalating on "merge never happened"?

## 1. Live sanity check I already ran (AC1)

`GET /repos/simple-container-com/api/rules/branches/main` (ruleset `9144757`), fetched this turn, `200 OK`, `Date: Sat, 17 Oct 2026 16:38:54 GMT`:
- `pull_request`: `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `dismiss_stale_reviews_on_push: true`, `allowed_merge_methods: ["squash"]`
- `required_status_checks`: 7 named CI jobs (Build Setup, Build cloud-helpers, Build github-actions, Build github-actions-staging, Build sc darwin/amd64+arm64, Run tests)
- `required_signatures` present
- `deletion` + `non_fast_forward` rules present

**Unchanged from every prior fetch cited in the item's History** — confirms AC1. This rules out "ruleset changed under us" as a confound for the observation we're about to make.

## 2. Target trivial fix — found a genuinely stale doc, not an invented typo

I did a scan for actual typos across `api` (recieve/seperate/occured/existant/accross/wheter/paramter/intial/definately/succesful/retreive/lenght/wich/teh/accomodate/neccessary/calender/informations/retuns/Usefull/priviledge/overwritting/repsonse) — **zero hits**. The codebase is clean; inventing a fake typo to "fix" would violate the brief's "content-correct" requirement.

Instead I found a **real, content-correct documentation staleness** that is itself relevant to this very item: `docs/CONTRIBUTING.md` currently reads:

> "Code review is **required** — current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land."

The live ruleset fetched above shows `required_approving_review_count: 2` **already in effect**, plus `require_code_owner_review: true` — i.e. the "will increase to 2" future-tense claim is stale; it already happened. This is exactly the class of fix the brief wants: zero behavioral risk, pure prose correction, and it's independently true (verified live, not asserted).

**MUST** — Developer edits `docs/CONTRIBUTING.md`, the sentence above, to:
> "Code review is **required** — branch protection enforces 2 required approving reviews plus a code-owner review; nothing self-merges here."

Nothing else in that file changes. No code, no schema, no CI config.

## 3. Scope decisions

| Item | Call |
|---|---|
| Fix the stale "1 reviewer" claim in `docs/CONTRIBUTING.md` | **MUST** — the vehicle for the proof |
| Let the SDLC orchestrator open the PR against `api` with `mergeStrategy: auto-merge` (not us manually opening via API) | **MUST** — this is the exact mechanism under test; a manually-opened PR would not exercise `maybeCreatePullRequest`/`autoMergeBlockedByReviewRequirement` |
| QA/DevOps record `mergeable_state`, PR URL, and whether the run escalated or completed cleanly | **MUST** |
| Touching any Go source, test, or CI config | **CUT** — brief explicitly forbids "risky or large changes"; this item's whole point is observing merge behavior on a trivial diff |
| Re-verifying the ruleset a second time later in the chain | **NICE-TO-HAVE** — one live check (done above) is sufficient per AC1's wording ("sanity check only, not new work"); QA/DevOps may re-check for free if convenient but shouldn't burn a dedicated step on it |
| Flipping `status: shipped` in the roadmap item | **conditional MUST** — only if the observed outcome is clean (no spurious escalation); otherwise stays `code-complete` with the gap recorded verbatim |

## 4. Acceptance criteria verification (plan-level — operator's AC1–AC6)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `api` ruleset confirmed unchanged (live GET) | **PASS** | `GET /repos/simple-container-com/api/rules/branches/main` → 200, `required_approving_review_count:2`, `require_code_owner_review:true`, ruleset id `9144757` — fetched this turn, §1 above |
| AC2 | Trivial, zero-risk PR opened against `api` | **PENDING — Developer/orchestrator** | Target identified: `docs/CONTRIBUTING.md` stale reviewer-count sentence (§2). Developer commits; SDLC engine opens the PR with `mergeStrategy: auto-merge` |
| AC3 | Observed `mergeable_state` + merge/escalation outcome recorded with evidence | **PENDING — QA** | QA must `GET /repos/simple-container-com/api/pulls/<N>` post-open and capture `mergeable_state` + `merged`/`merged_at` |
| AC4 | Run completes WITHOUT spurious escalation (or records precisely why if it does) | **PENDING — QA/DevOps** | Expected: `mergeable_state: "blocked"` (0/2 reviews), fix recognizes the review-requiring ruleset, falls back to a normal PR-for-human-review completion rather than alarming |
| AC5 | Roadmap item file + README updated to match outcome | **PENDING — DevOps**, conditional | Flip `code-complete`→`shipped` ONLY if AC4 is clean; otherwise leave `code-complete` and record the precise gap in `## History` |
| AC6 | `docs/roadmap/lint.sh` passes (0 errors) after the roadmap edit | **PENDING — DevOps** | Run `bash docs/roadmap/lint.sh` in `forge` after any roadmap-doc edit, before signoff |

## 5. Handoff guidance for Architect

- No architectural design needed — this is a one-line doc fix plus an observation. Architect's job here is thin: confirm the `docs/CONTRIBUTING.md` edit is the right trivial vehicle (or pick an equally trivial alternative if Architect finds a better zero-risk candidate), and hand to Developer with the instruction to let the **normal SDLC PR-open path** run (do not hand-open the PR via `api_call` — that bypasses the exact code path under test).
- Developer: make the one-line edit, commit on this run's branch, done. Do not touch Go code, CI, or anything under `pkg/`/`cmd/`.
- QA: the live observation is the actual deliverable. Fetch the opened PR's `mergeable_state` via the GitHub API and quote it verbatim (not paraphrased) in the QA handoff, plus the PR URL.
- DevOps: owns the roadmap-doc write to `forge`'s `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` (`## Status`/`## History`) and `docs/roadmap/README.md` index row, gated on AC4's outcome, then run `lint.sh`.
- `slice_id`: `api-repo-branch-protection-blocks-sdlc-automerge` (for downstream next-slice resolution).

## 6. Commit + branch identity

No code mutations in this turn — PM plan only.

## 7. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | (empty — PM turn produces no file mutations) |

## 8. Tests run

N/A — no code changed this turn. Developer's turn should run `go vet` scoped (`make check` per `api`'s `CLAUDE.md` — memory caps apply, do NOT run unbounded `go build ./...`); since the only change is a markdown file, no Go test run is required, but CI's required status checks (7 named jobs per the live ruleset) will still gate the PR regardless of file type — that's expected and is part of what we're observing, not a defect to fix.

**Verdict:** signoff