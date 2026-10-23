[thinking] # Architect Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` — Design confirmation for live positive-path proof

## 0. Role scope this turn

This is a **design/validation** turn, not an implementation turn. PM proposed a trivial, zero-risk vehicle (a stale-prose fix in `docs/CONTRIBUTING.md`); my job is to verify that vehicle is sound, verify it's still content-correct against a fresh live fetch, and hand Developer an exact, unambiguous instruction so the SDLC's normal PR-open path (not a hand-opened API call) is what gets exercised.

## 1. Fresh verification (independent of PM's fetch)

**Ruleset re-check** — `GET https://api.github.com/repos/simple-container-com/api/rules/branches/main`, this turn, `200 OK`, `Date: Sat, 17 Oct 2026 16:44:23 GMT`:
- `pull_request` rule (ruleset `9144757`): `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `dismiss_stale_reviews_on_push: true`, `required_review_thread_resolution: true`, `allowed_merge_methods: ["squash"]`
- `required_status_checks`: 7 named CI contexts (Build Setup, Build cloud-helpers, Build github-actions, Build github-actions-staging, Build sc darwin/amd64, Build sc darwin/arm64, Run tests)
- `required_signatures`, `deletion`, `non_fast_forward` rules present

**Byte-for-byte unchanged from PM's 16:38:54 GMT fetch** (same `required_approving_review_count:2` + `require_code_owner_review:true`) → re-confirms AC1 independently.

**File content re-check** — `read_repository_file(docs/CONTRIBUTING.md)` this turn, `sha: 05dd5173087fee8d79a2966e14ad6f56f4469342`. The exact sentence PM quoted is present verbatim in the live file:

> "Code review is **required** — current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land."

This confirms: (a) PM did not misquote, (b) the staleness claim is real — the live ruleset shows `required_approving_review_count: 2` already enforced today, not as a future state, plus a `require_code_owner_review` gate the sentence doesn't mention at all.

## 2. Design verdict: PM's vehicle is approved, no better candidate found

I considered whether a different trivial target would be structurally cleaner (e.g. a comment in Go source vs. a docs file) — rejected: touching any `.go` file, however trivial, pulls in the 7 required CI status checks as a *meaningful* gate rather than a formality, and risks an unrelated flaky-CI escalation confounding the observation we actually want (review-gate escalation behavior). A markdown-only diff keeps the CI-check dimension as pure signal (the checks still have to pass/run, per PM's note, but there's zero chance of a Go-level regression muddying AC4's verdict). **Confirmed: `docs/CONTRIBUTING.md` is the right, and only necessary, vehicle.**

## 3. Exact diff for Developer (verbatim — do not paraphrase)

In `docs/CONTRIBUTING.md`, under `### Pull request`, replace this line:

```
- Code review is **required** — current branch protection enforces 1
  reviewer minimum; this will increase to 2 once admin-UI gates land.
```

with:

```
- Code review is **required** — branch protection enforces 2 required
  approving reviews plus a code-owner review; nothing self-merges here.
```

No other line in the file changes. This is a single paragraph edit, zero behavioral/code risk.

## 4. Process guidance (binding on Developer/DevOps downstream)

- Developer: commit this one edit on the run's branch for `simple-container-com/api`, using normal commit conventions from this same file (Conventional Commits subject, e.g. `docs: correct stale reviewer-count claim in CONTRIBUTING.md`). Do **not** touch any `.go`, `pkg/`, `cmd/`, or CI-config file.
- Whoever triggers the PR-open step: let the **SDLC orchestrator's normal `maybeCreatePullRequest` path** open the PR with `mergeStrategy: auto-merge` set on the dispatch. Do **not** manually open the PR via `api_call`/`gh pr create` — that bypasses `autoMergeBlockedByReviewRequirement`, which is the exact code path this whole dispatch exists to observe.
- QA: fetch the opened PR via `GET /repos/simple-container-com/api/pulls/<N>` and quote `mergeable_state` verbatim (expect `"blocked"` given 0/2 required approvals + missing code-owner review), plus `merged`/`merged_at` (expect `false`/`null`). Also capture whether the run's own terminal status is a clean "awaiting human review" completion vs. an escalation alarm.
- DevOps: gate the roadmap-doc flip (`code-complete`→`shipped`) strictly on QA's AC4 verdict being clean; otherwise leave status `code-complete` and record the gap verbatim in `## History`. Run `docs/roadmap/lint.sh` in `forge` after the edit, 0 errors required.
- `slice_id`: `api-repo-branch-protection-blocks-sdlc-automerge`.

## 5. Commit + branch identity

No code mutations in this turn.

## 6. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | (empty — Architect turn, design/validation only, no mutations) |

## 7. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `api`'s branch-protection ruleset confirmed unchanged (live GET) | PASS | `GET /repos/simple-container-com/api/rules/branches/main` → 200 OK, `Date: Sat, 17 Oct 2026 16:44:23 GMT`, `required_approving_review_count:2`, `require_code_owner_review:true`, ruleset id `9144757` — byte-identical to PM's independent fetch at 16:38:54 GMT (§1 above) |
| AC2 | Trivial, zero-risk PR opened against `api` | PENDING — Developer/orchestrator | Exact diff specified in §3; vehicle validated against live file content (`sha: 05dd5173087fee8d79a2966e14ad6f56f4469342`), confirmed content-correct against live ruleset |
| AC3 | Observed `mergeable_state` + merge/escalation outcome recorded with evidence | PENDING — QA | Instruction given in §4; not yet executed this turn |
| AC4 | Run completes WITHOUT spurious escalation (or records precisely why if it does) | PENDING — QA/DevOps | Design keeps risk surface to a single markdown paragraph specifically to isolate this signal (§2) |
| AC5 | Roadmap item + README updated to match outcome | PENDING — DevOps, conditional on AC4 | Gating instruction given in §4 |
| AC6 | `docs/roadmap/lint.sh` passes (0 errors) after roadmap edit | PENDING — DevOps | Instruction given in §4 |

## 8. Tests run

N/A this turn — no code mutated. Developer's turn: markdown-only change, no Go test run required; CI's 7 required status-check jobs will still run and must pass as part of what's being observed (not a defect to pre-empt).

**Empty-PR-OK:** not applicable this turn — this is a design/validation handoff feeding a downstream turn that WILL produce a real source diff (the CONTRIBUTING.md edit + PR); it is not itself the terminal verification-only turn.

**Verdict:** signoff