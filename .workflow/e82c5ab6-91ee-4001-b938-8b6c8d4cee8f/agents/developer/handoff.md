Work is committed, pushed, and verified. Here is my handoff.

---

# Developer Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` — live positive-path proof vehicle

## 1. Commit + branch identity

- **Commit SHA:** `5884d394c4d30b6f633f56c360fa4c27fc647a65` (short `5884d394`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/e82c5ab6-91ee-4001-b938-8b6c8d4cee8f`
- **Pushed:** yes — `43cacc59..5884d394`, exit 0, branch in sync with origin (no ahead/behind markers)
- **PR URL:** none yet. No PR exists for this branch (`list_pull_requests` with `head=<this branch>` → zero matches). The engine auto-opens it at handoff time; per Architect §4 I deliberately did **not** hand-open it, since doing so bypasses `maybeCreatePullRequest` / `autoMergeBlockedByReviewRequirement` — the exact code path under observation.
- **DCO:** `Signed-off-by: David Black (developer) <forge-agent+developer@simple-forge.com>`, trailer matches author.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/CONTRIBUTING.md` | +2/-2 | Lines 113-114: replaced the stale "current branch protection enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land" with "branch protection enforces 2 required approving reviews plus a code-owner review; nothing self-merges here" |

One file, 2 insertions / 2 deletions. No `.go`, `pkg/`, `cmd/`, schema, or CI file touched — verified: `git show --name-only HEAD | grep -E '\.go$|^cmd/|^pkg/'` → **NONE**.

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | `api` ruleset confirmed unchanged (live GET) | **PASS** | `GET /repos/simple-container-com/api/rules/branches/main` → 200 OK, `Date: Sat, 17 Oct 2026 17:05:10 GMT`, `X-Github-Request-Id 92FC:24A618:623937:9A522B:6AD3AAC6`. Ruleset `9144757`: `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `required_review_thread_resolution: true`, `allowed_merge_methods: ["squash"]`, 7 required status checks, `required_signatures`. Matches PM's 16:38:54Z and Architect's 16:44:23Z fetches. **New detail neither prior turn reported:** `require_extra_approval_for_unattributed_changes: true` is also present — relevant, since a bot-authored commit is a prime candidate for "unattributed". |
| AC2 | Trivial, zero-risk PR opened against `api` by this run | **PARTIAL — commit done, PR open is engine-owned** | Commit `5884d394` pushed to the run branch. Verification gate run: `git log -1 --format="%H %an" -- docs/CONTRIBUTING.md` → `5884d394… David Black (developer)` — my commit, this run, on the real project path. `git diff origin/main...HEAD -- . ':(exclude).workflow'` → 1 file, +2/-2, so the branch carries a **real source diff**, not just engine metadata. PR itself not yet open — correctly deferred to the engine (see §1). |
| AC3 | Observed `mergeable_state` + merge/escalation outcome recorded | **FAIL-blocked — QA/engine-owned, cannot be observed from this turn** | Unobservable by construction: the PR does not exist until my handoff is committed, so no `mergeable_state` for *this* branch can exist while I am running. I did capture two live structural analogues instead — see §6. Exact call for QA: `GET /repos/simple-container-com/api/pulls/<N>`. |
| AC4 | Run completes WITHOUT spurious auto-merge escalation | **FAIL-blocked — terminal-state AC, resolvable only after my turn ends** | Same reason as AC3: the escalation decision is made by the engine's merge-evidence gate after the chain terminates. Predicted outcome + the two confounds that could still break it are in §6-§7. |
| AC5 | `forge` roadmap item + README updated to match outcome | **FAIL-blocked — DevOps-owned, and gated on AC4 which is not yet known** | Correctly not attempted: flipping to `shipped` now would be fabricating AC4's result. Write access confirmed reachable (full `git clone` of `forge` succeeded this turn, exit 0 — the `permission: none` collaborator-API reading is the known App-token false negative). Exact coordinates handed over in §8. |
| AC6 | `docs/roadmap/lint.sh` passes (0 errors) after roadmap edit | **PASS (pre-edit baseline established)** | Ran on a **full clone** of `forge`: `403 items, 403 index rows, README.md 231 lines — 0 errors, 0 warnings`, exit 0. This is the number DevOps must preserve. **Procedural warning in §8** — a sparse/shallow clone fabricates hundreds of phantom errors. |

## 4. Tests run

No Go source was touched, so no Go build or test is applicable — and per `CLAUDE.md` an unbounded `go build ./...` on this module is an OOM hazard (single `compile` processes measured >6 GB), so running one to "prove" a markdown edit would be actively reckless. What I ran instead:

```
$ git show --name-only --format="" HEAD | grep -E '\.go$|^cmd/|^pkg/'
NONE — markdown only
```

```
$ grep -rn -iE "markdownlint|vale|mdl|prettier|remark|typos|codespell" .github/workflows/ welder.yaml .golangci.yml
(no matches — no prose/markdown linter in CI)
```

```
$ grep -n -i "contributing" docs/mkdocs.yml
CONTRIBUTING.md not in mkdocs.yml nav   # not in the docs build either
```

```
$ bash docs/roadmap/lint.sh        # in a FULL clone of simple-container-com/forge
403 items, 403 index rows, README.md 231 lines — 0 errors, 0 warnings
```

**Explicitly skipped:** `make check` (`go vet ./...`). Justification: the commit contains zero Go files, so vet's result is identical to `main`'s and the ~3 min / 2.3 GB spend buys no signal. The 7 required CI status checks will still run against the PR regardless of file type — that is expected and is part of what AC3/AC4 observe, not a defect to pre-empt.

## 5. Content-correctness of the edit (the part that must not be hand-waved)

The brief demands a "content-correct" fix, so the new sentence is itself a factual claim that had to be verified, not asserted:

- "**2 required approving reviews**" ← `required_approving_review_count: 2`, live fetch 17:05:10Z
- "**plus a code-owner review**" ← `require_code_owner_review: true`, same fetch
- "**nothing self-merges here**" ← `current_user_can_bypass: "never"` (recorded in the item's Status table) and `allowed_merge_methods: ["squash"]` with the review gate active

I also independently confirmed the pre-edit blob was `05dd5173` — **byte-identical to the sha the Architect read** — so no third party had drifted the file between their turn and mine, and PM's quote was not a paraphrase.

## 6. Live evidence I *could* gather for AC3/AC4 — two structural analogues

AC3/AC4 are unobservable from inside my own turn, but I did not want to leave them as bare "pending". I fetched two currently-open PRs against `api` that bracket the question:

**PR #448** — `docs(vm): the byo-host example describes the landed Caddy/TLS path`, fetched 17:08:45Z. This is the closest possible analogue to my change: **docs-only, 3 files, +102/-11, zero Go**.
```
"mergeable": true,  "mergeable_state": "blocked",
"merged": false,    "merged_at": null,  "auto_merge": null
```
This is the predicted terminal state for my PR, now confirmed on a real docs-only PR against `api` *today*: GitHub says "would merge, but a required rule hasn't cleared". `mergeable: true` is the important half — it proves the block is the **review gate**, not a content/conflict defect.

**PR #435** — the bot PR from this same workflow (run `9770a6e1`), fetched 17:08:04Z:
```
"mergeable": null,  "mergeable_state": "unknown",
"merged": false,    "user.login": "simple-container-forge[bot]"
"body": "<!-- forge:escalation --> ⚠️ Run escalated — operator review required
         Escalation reason: merge-evidence gate … auto-merge did not land"
```
This is the **pre-fix failure mode preserved in amber** — exactly the escalation this item exists to eliminate. Note `mergeable_state: "unknown"`, not `"blocked"`: GitHub returns `unknown` while mergeability is still being computed (and #435's head is 5 days stale). **This matters for QA**: a single poll that catches `"unknown"` must not be read as either success or failure. Poll until it settles to `blocked`/`clean`/`dirty`.

## 7. Two confounds QA/DevOps must not mistake for the fix failing

Both are independent of the review gate and would each block a merge on their own. If either fires, AC4 must be recorded as a *distinct* cause, not as the review-gate regression:

1. **`required_signatures` vs. unsigned commits.** The ruleset requires signed commits. Every engine-authored commit on this branch is **unsigned** — `git log --format="%G?"` returns `N` for `43cacc59`, `d1015aa3`, `ba6d98ad`, and `gpg` is not even installed in this workspace (`error: cannot run gpg: No such file or directory`). My own commit is likewise unsigned; I used `-c commit.gpgsign=false` deliberately rather than failing the commit, matching the engine's existing convention on this branch. So **even a fully-approved PR on this branch cannot squash-merge** until signing is addressed. `docs/CONTRIBUTING.md:94` documents this requirement — note the irony that it sits ten lines from the sentence I corrected. This is a real, separate gap worth filing.
2. **`require_extra_approval_for_unattributed_changes: true`** (my fetch; unreported by PM/Architect). A bot-authored commit plausibly counts as unattributed, which would raise the effective approval bar above 2.

Neither is a defect in the deployed `autoMergeBlockedByReviewRequirement` fix. The fix's job is to recognise the review requirement and *not alarm*; it is not expected to make this PR merge.

## 8. Handoff to QA, then DevOps

**QA** — AC3/AC4 are yours:
- `GET /repos/simple-container-com/api/pulls/<N>` for the PR opened from this branch. Quote `mergeable_state`, `mergeable`, `merged`, `merged_at`, `auto_merge` verbatim. **Poll until `mergeable_state` is not `"unknown"`** (see §6).
- Expected clean result: `mergeable: true`, `mergeable_state: "blocked"`, `merged: false`, and the run terminating as *awaiting human review* — **no** `<!-- forge:escalation -->` body like #435's.
- Verify the PR body does **not** carry the escalation marker. That single string is the cleanest pass/fail discriminator available.

**DevOps** — AC5/AC6, gated strictly on QA's AC4 verdict:
- Item file: `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` (blob `e62e9923b8d563db68edae3850ea53c5c1f659e1`, 23,668 bytes), front-matter currently `status: in-progress`, `dispatched: "2026-10-17"`, `repo: api`.
- README index row: `docs/roadmap/README.md:155`, currently `| 🚧 IN PROGRESS | … | api | auto | …`.
- `lint.sh:120` enforces that the README status cell **exactly matches** the front-matter — a `shipped` flip requires the cell to become `✅ SHIPPED` (vocabulary at `lint.sh:14`). Change one without the other and AC6 fails.
- ⚠️ **Run `lint.sh` only in a FULL clone.** I measured this directly: a `--sparse` checkout of `docs/roadmap` alone reports **418 errors**; adding `docs/design` drops it to **15**; a plain full clone reports **0 errors, 0 warnings**. Every one of those 418 was a phantom `../../design/...` dangling link. Do not report a sparse-clone number as a regression.
- Record `mergeable_state` + PR URL + escalation-or-clean into `## Status` and `## History`. Flip to `shipped` **only** if AC4 is clean. If the signature gate (§7.1) blocks the merge, that is a *new* finding — leave `code-complete` and record it precisely.

## 9. Scope discipline

I did not touch Go source, CI config, schemas, or the ruleset; I did not hand-open the PR; and I did not pre-emptively flip the roadmap status to `shipped`. The last one is the one worth stating plainly: AC5's flip depends on AC4's outcome, and AC4 cannot be known until after this turn ends. Writing `shipped` now would have made the AC table read all-PASS at the cost of being fabricated — which is precisely the failure this item has been chasing for seven dispatches.

**Empty-PR-OK:** no — this run carries a real source diff against `main` (`docs/CONTRIBUTING.md`, +2/-2, commit `5884d394`), so the empty-diff escalation gate does not apply and the declaration would be inappropriate.

**Verdict:** signoff