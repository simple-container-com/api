All evidence gathered and both repos are in their final state.

# DevOps Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` (6th dispatch)

## 0. Headline

This is the **first dispatch of this item to reach the DevOps character**. I did not get the hand-observed merge decision the brief asked for — and I found out *why no dispatch ever could*. In exchange I obtained something strictly stronger than a single observation: a **byte-identity proof** that the deployed predicate's answer against `api` is deterministic.

The honest framing: the brief's AC5 as literally worded is **architecturally unreachable from any character turn**. I am recording that as the finding rather than dressing an inference up as an observation.

## 1. Commit + branch identity

Two repos, as the brief and PM both required — and they are **not** symmetrical:

- **`simple-container-com/api`** (this run's branch): **no mutations by me.** The only source commit is the Developer's `a1a3990b84055a0b8b823d18345104fccc5c5f44`. I deliberately did not widen this run's diff — the PR *is* the experimental apparatus, and adding a DevOps commit would have changed the artifact under test.
- **`simple-container-com/forge`** (roadmap): commit **`e2f70b33ad78ad1c635ce2ecad33a6b06b2357b6`** on branch `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/2e40aa56-b159-48dd-85c3-aa71844f4b08`, opened as **PR [#1615](https://github.com/simple-container-com/forge/pull/1615)** (open, 1 file, +28/-2, DCO trailer present).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `forge:docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +28/-2 | `next:` rewritten to a read-not-dispatch instruction; Positive-path-proof Status row replaced with this run's byte-identity evidence; new `## History` entry incl. the correction to the item's standing diagnosis. `status:` left `in-progress` — see §5. |

No files touched in `api`. `git status --porcelain` there shows only the engine's untracked `.forge-prompt.md`.

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Ruleset `9144757` still active, `required_approving_review_count: 2` | **PASS** | Two independent live fetches, mine: `GET /rulesets/9144757` → 200, `Date: Sun, 18 Oct 2026 01:15:07 GMT`, req-id `A4A0:3D3C43:1350761C:1D42B975:6AD41D9A`; **and** `GET /rules/branches/main` (the endpoint the gate actually calls) → 200, `01:15:35 GMT`, req-id `EC70:3B6CF4:13CA673F:1E035833:6AD41DAC`. Both: `enforcement:"active"`, `required_approving_review_count:2`, `require_code_owner_review:true`, `bypass_actors:[]`, `current_user_can_bypass:"never"`, `updated_at:2026-06-30T19:07:03Z`. |
| AC2 | Developer's change trivial, docs-only, on this run's branch | **PASS** | `git diff main...HEAD --numstat -- . ':(exclude).workflow'` → exactly `2 2 docs/CONTRIBUTING.md`. `git log -1 -- docs/CONTRIBUTING.md` → `a1a3990b David Black (developer)`. Unchanged by my turn. |
| AC3 | PR opens against `api:main` from this run's branch | **FAIL-blocked (engine-ordered, not a defect)** | `list_pull_requests(head=…run/2e40aa56…, state=all)` → **no PR exists yet**. `maybeCreatePullRequest` runs inside `completeRun`, i.e. *after* my handoff. Newest `api` PRs are #450–#453, none mine. Cannot be made to PASS from this turn by any means. |
| AC4 | `autoMergeWithBackoff` attempts merge and cannot satisfy the gate | **FAIL-blocked → and the premise is wrong** | Not merely unobservable: per the deployed source, `autoMergeWithBackoff` is **never called** on this path. `orchestrator_pr.go` branches *before* it — `else if o.autoMergeBlockedByReviewRequirement(...)` skips straight past the merge attempt. The brief expected a burned ~33 s backoff budget; the fix's whole point is that it no longer happens. Recording the AC as mis-specified rather than silently "verified". |
| AC5 | `autoMergeBlockedByReviewRequirement` fires; run ends as a clean open PR | **FAIL-blocked (structurally unreachable) — but proven deterministic** | See §4. Direct observation requires reading state the engine writes after the last handoff. What I *can* prove: the live payload is **byte-identical** (1567 B, `sha256 9c7016c205e531ac…`) to the fixture the **currently-deployed** code asserts `required == true` on. Identical input + identical binary ⇒ deterministic, not probable. |
| AC6 | Orchestration-layer bug kept distinct from a fix defect | **PASS** | The `e82c5ab6` fault (`condition_blocked(transition=qa-to-devops): verdictResolved=<unparseable>`, resumeCount 14) **did not recur**: run timeline shows pm→architect→developer→qa→devops with every validate `verdict: advance`. It stays a separate defect. I also add a **third** bucket not in the brief: AC3–AC5 are blocked by *engine phase ordering*, which is neither an orchestration bug nor a disproof of the fix. |
| AC7 | Roadmap `## Status`/`## History` updated; flip to `shipped` only if AC3–AC5 hold | **PASS** | PR #1615, commit `e2f70b33ad`. `status:` correctly held at `in-progress` because AC3–AC5 did not hold cleanly. Flipping to `shipped` would have been exactly the over-claim the brief forbids. |

## 4. The finding: why six dispatches "never observed" this

The brief, and five prior History entries, treat "DevOps never ran" as the blocker. That diagnosis is **incomplete**, and acting on it produced six dispatches chasing an impossible observation.

From the deployed source at `802aa772330e`, the fallback lives inside `completeRun`, which the engine invokes **after** the final character hands off:

```
completeRun(...)                              ← runs AFTER my handoff
  ├─ maybeCreatePullRequest(...)              ← AC3's PR is created HERE
  │    └─ if MergeStrategyAutoMerge:
  │         else if autoMergeBlockedByReviewRequirement(...)   ← AC5 decision
  │              SetAutoMergeSkippedReviewRequired(true)       ← flag stamped
  └─ maybeEscalateCommitNotOnMain(...)        ← reads the flag, returns false
```

**No character can observe its own run's merge decision.** Reaching DevOps was necessary but never sufficient. The roadmap entry now says this plainly so a 7th dispatch isn't spent on it.

**What I proved instead.** Rather than infer, I verified identity of input and binary:

```
live  GET /repos/simple-container-com/api/rules/branches/main  → 1567 bytes
      sha256 9c7016c205e531ac2a9373c16bafd9d4dd33434d12c4ef38d9d1e7b113744548
fixture liveAPIRulesResponse @ 802aa772330e                     → 1567 bytes
      sha256 9c7016c205e531ac2a9373c16bafd9d4dd33434d12c4ef38d9d1e7b113744548
  *** fixture[0] is BYTE-IDENTICAL to today's live payload ***
```

That fixture's assertion, read verbatim from the deployed tree:

```go
assert.True(t, required,
  "the live simple-container-com/api ruleset (9144757) must be detected as review-requiring; "+
  "a false here means autoMergeBlockedByReviewRequirement stops firing and api dispatches escalate again")
```

And the deployed binary is that code: `forge-conductor` main = `802aa772330e`, deploy run `39056975156`, `conclusion: success`, `2026-10-17T18:17:58Z`. The predicate it runs:

```go
if r.Parameters.RequiredApprovingReviewCount >= 1 || r.Parameters.RequireCodeOwnerReview {
    return true, nil      // live: 2, and true — both arms satisfied
}
```

Same bytes in, same binary ⇒ the answer is **deterministic**. I consider this stronger evidence than one lucky observation, which is why I pursued it once I established the observation was unreachable.

## 5. Why I did not flip to `shipped`

The brief authorises `shipped` only if the live-dispatch clause closes cleanly. It did not — AC3–AC5 are FAIL-blocked. Asserting `shipped` would mean claiming a terminal state I cannot read from inside the run, which is the precise failure mode the item's own history already records twice. Held at `in-progress`, with the closing condition written as a **read, not a dispatch**:

> run `2e40aa56` ends `completed` with `autoMergeSkippedReviewRequired: true`, and its `api` PR is open with no `[escalated]` prefix → `shipped`. If it escalates on merge evidence **despite** the flag, that is a genuine regression and should reopen the item.

That is a ~30-second check for the next Driver pass instead of a 7th dispatch.

## 6. Tests run

```
$ bash docs/roadmap/lint.sh          # forge @ e2f70b33ad, docs/ fully checked out
406 items, 406 index rows, README.md 234 lines — 0 errors, 0 warnings
```

First run of this showed `420 errors — dangling link ../../design/...`; that was **my sparse-checkout excluding `docs/design`**, not a content fault. Re-ran with `git sparse-checkout set docs` → clean. Reporting both so the 420 isn't mistaken for a real regression later.

```
$ python3  # byte-compare live payload vs committed fixture
live sha256   : 9c7016c205e531ac2a9373c16bafd9d4dd33434d12c4ef38d9d1e7b113744548  (1567 B)
fixture[0]    : 9c7016c205e531ac2a9373c16bafd9d4dd33434d12c4ef38d9d1e7b113744548  (1567 B)
  *** BYTE-IDENTICAL ***
```

**Go tests not run — toolchain absent in my sandbox** (`go: command not found`; `MemTotal: 12.0 GB`, `gh` present). I scaffolded an executable harness at `/tmp/ghdemux` that replays the deployed predicate against the live payload via `httptest`, then could not compile it. I am declaring that rather than claiming a pass — and note the byte-identity proof makes execution redundant: the deployed CI already runs that exact assertion on that exact input. This also **confirms QA's finding (a)**: sandboxes differ per character (Go present for QA at 3.7 GB, absent for Developer and for me), so an upstream "toolchain absent/present" claim must be re-probed, never inherited.

`make check` not applicable — I changed one markdown file in a different repo, zero Go surface.

## 7. Carried forward (unfixed, deliberately out of scope)

1. **`docs/ARCHITECTURE.md:79`** still understates the control as "≥1 reviewer" in the Trust-boundaries table. Verified by both Developer and QA. Not fixed: widening this run's diff would have altered the artifact under test. Worth filing — and it is a ready-made payload for any future `api` dispatch.
2. **Unsigned commits + `required_signatures`.** Developer's `a1a3990b84` is unsigned (gpg/ssh-keygen absent). The reasoning that this won't confound the result holds and I verified its basis: `allowed_merge_methods:["squash"]`, and `main`'s merge commits show `committer: GitHub <noreply@github.com>` with `verification.verified: true` — GitHub re-signs server-side at merge. Since auto-merge is now *skipped* rather than attempted, the signature gate is moot for this run either way.
3. **AC4 is mis-specified** for the post-fix world (see table). Any future dispatch of this item should drop the "backoff budget burned" expectation.

PR #450 (run `e82c5ab6`'s open draft) **left untouched** — operator's call, as instructed.

## 8. Operator action required

One read, no dispatch: confirm run `2e40aa56`'s terminal `status` and `autoMergeSkippedReviewRequired`, plus its `api` PR title. Then merge `forge` PR #1615 (it needs 2 reviews there too) and flip `status:` per the condition in §5.

**Empty-PR-OK:** no — this run carries a real source change (`api:docs/CONTRIBUTING.md`, +2/-2) outside `.workflow/`, which is exactly what the auto-merge gate needs to act on, plus a real roadmap change in `forge` (+28/-2).

**Verdict:** signoff