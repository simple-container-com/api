Everything verified. The work is committed to the run branch with a real SHA, the diff is exactly single-hunk and docs-only, and the factual claim I wrote matches the live ruleset I fetched myself.

# Developer Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` (6th dispatch)

## 1. Commit + branch identity

- **Commit SHA:** `a1a3990b84055a0b8b823d18345104fccc5c5f44` (short `a1a3990b84`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/2e40aa56-b159-48dd-85c3-aa71844f4b08`
- **Repo:** `simple-container-com/api`, base `main`
- **PR URL:** not yet opened — engine auto-opens at handoff time. DevOps must record the number.
- **DCO:** `Signed-off-by: David Black (developer) <forge-agent+developer@simple-forge.com>` — verified to match the commit author exactly (`.github/workflows/dco.yml` compares trailer to author; a mismatch fails the PR).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/CONTRIBUTING.md` | +2/-2 | Lines 113–114, single hunk under `### Pull request`: replaced the false "enforces 1 reviewer minimum; this will increase to 2 once admin-UI gates land" with "enforces 2 approving reviews plus code-owner review before merge; nothing self-merges here" |

That is the complete non-`.workflow` diff for this run — confirmed by `git diff main...HEAD --numstat -- . ':(exclude).workflow'` → exactly `2 2 docs/CONTRIBUTING.md`. No Go files, no `CODEOWNERS`, no CI config.

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Ruleset `9144757` still active, `required_approving_review_count: 2` | PASS | My own live `GET /repos/simple-container-com/api/rulesets/9144757` → 200 at `Date: Sun, 18 Oct 2026 00:40:51 GMT`, ETag `W/"10afdbdf28aa…"`. `enforcement: "active"`; `pull_request` rule `required_approving_review_count: 2`, `require_code_owner_review: true`, `require_last_push_approval: true`, `required_review_thread_resolution: true`, `require_extra_approval_for_unattributed_changes: true`, `allowed_merge_methods: ["squash"]`; plus `required_status_checks` (7 contexts, `strict_required_status_checks_policy: true`), `required_signatures`; `bypass_actors: []`, `current_user_can_bypass: "never"`. **Note `updated_at: 2026-06-30T19:07:03Z`** — the ruleset has not been touched since well before any dispatch of this item, so it is stable ground truth, not a race. |
| AC2 | Change is genuinely trivial (docs-only) and lands as a commit on this run's branch | PASS | Commit `a1a3990b84`; `git diff --numstat` → `2 2 docs/CONTRIBUTING.md`, one hunk at `@@ -110,8 +110,8 @@`. Verification gate: `git log -1 -- docs/CONTRIBUTING.md` → `a1a3990b84 David Black (developer)` — my commit, this run. Working tree clean. |
| AC3 | PR opens against `api:main` from this run's branch | FAIL-blocked (not mine to do) | Engine auto-opens the PR at handoff. Branch is pushed and tracking `origin/forge/workflow/…/run/2e40aa56-…`. DevOps owns recording the PR number. |
| AC4 | `autoMergeWithBackoff` attempts merge, cannot satisfy review gate (0/2) | FAIL-blocked (downstream) | Not observable from a Developer turn — requires the orchestrator's merge attempt. Structurally pre-determined though: `bypass_actors: []` + `current_user_can_bypass: "never"` means no App token can bypass, so 0/2 reviews is guaranteed at merge time. |
| AC5 | `autoMergeBlockedByReviewRequirement` fires; run ends as a normal open PR, not an escalation | FAIL-blocked (downstream) | DevOps turn owns this. It is the crux observation 5 prior dispatches never reached. |
| AC6 | If something else breaks the chain, name it as a distinct defect | PASS (for my turn) | Nothing broke in my turn — PM→Architect→Developer is clean, no validator issue hit. I did hit two **environment** limitations which I am recording explicitly as *environment*, NOT as defects in the branch-protection fix — see §6. |
| AC7 | Roadmap item `## Status`/`## History` updated in the `forge` repo | FAIL-blocked (not mine, and correctly so) | Architect explicitly instructed: do not update prematurely from a Developer turn before the merge-path outcome is observed. The outcome is still unobserved. Whoever reaches furthest (DevOps) owns it. Updating it now would be fabricating the result this run exists to measure. |

## 4. Tests run

**`make check` was NOT run — the Go toolchain is absent from this sandbox:**

```
$ go version
/bin/bash: line 1: go: command not found
$ du -sh "$(go env GOCACHE)"
GOCACHE missing/empty
```

Per the BUILD-BEFORE-HANDOFF rule I am declaring this rather than claiming a pass. Mitigation and why I judge the risk nil:

- The diff is two markdown lines. There is **zero** Go surface — no `.go` file, no `go.mod`, no struct tag, no annotated handler, so no `make swag` drift either. `go vet` cannot be affected by `docs/CONTRIBUTING.md`.
- `MemTotal: 11.7 GB` — below the ~16 GB floor `CLAUDE.md` sets for linking anyway, so `make build` would have been the wrong gate here even with Go present.
- Markdown checks I *could* run, and did:

```
$ ls .markdownlint* .mdlrc 2>/dev/null
--- md lint config: none found ---
$ awk 'NR>=113 && NR<=114 {print length($0)}' docs/CONTRIBUTING.md
72
72
```

No markdown linter is configured in this repo, and both new lines are 72 chars — consistent with the 46–68 char hand-wrapped neighbours in the same list, so the file's wrap style is preserved.

- CI will run the 7 required status checks on the PR regardless; a docs-only change has no mechanism to fail them.

## 5. Findings I'm escalating — one is a real defect in the brief's own plan

**(a) A second stale reference the Architect's plan missed.** The Architect scoped the scan to `docs/CONTRIBUTING.md` and concluded "No other mention exists elsewhere in the doc" — true for that file, but a repo-wide scan finds another:

```
$ grep -rn "1 reviewer\|reviewer minimum" --include="*.md" . | grep -v "^./.workflow/"
./docs/ARCHITECTURE.md:79: | **Dev → GH Actions** | … | Branch protection (signed commits required, ≥1 reviewer, DCO trailer enforced) |
```

`docs/ARCHITECTURE.md:79` states "≥1 reviewer" in the trust-boundary table. Strictly, "≥1" is not *false* when 2 are required — but it understates the control in a security document. **I deliberately did NOT fix it**, because PM and Architect both set a hard single-hunk scope guard, and widening the diff unilaterally would compromise the merge-path observation this run exists to make. Recommend filing it as a separate trivial docs item — it is also a ready-made payload for any future dispatch needing a safe `api` change.

**(b) The signed-commit risk the Architect flagged is real but misdirected.** Architect instructed me to `git commit -S -s`. That is **impossible in this sandbox**:

```
$ command -v gpg;        → gpg: NOT INSTALLED
$ command -v ssh-keygen; → ssh-keygen: NOT INSTALLED
$ ls ~/.ssh/             → ~/.ssh: absent
```

I committed with `-s` (DCO trailer, verified present and author-matching) but **unsigned**, because signing cannot be done here. Before concluding this confounds the experiment, note the evidence that it does not:

- The engine's own prior commits on this branch are **already unsigned** — checked via raw commit headers rather than `git log %G?`, which is unreliable with gpg missing:

```
0a008cde SIGNED   ci: route runs-on through vars.FORGE_CI_RUNNER (#449)
fba2b721 SIGNED   feat(k3s): single-node k3s on a byo host…
cd04c7e0 UNSIGNED forge: workflow …init
25d82963 UNSIGNED forge: character architect (Dan Johnson)…
```

- The signed commits on `main` are signed by **GitHub, not the author**: `committer=GitHub <noreply@github.com>`, single-parent (squash). GitHub re-signs server-side when it creates the squash commit, so `required_signatures` is satisfied *at merge time* by GitHub's key, not by the PR branch's commits.

**Implication for DevOps — this matters for reading the result correctly:** `required_signatures` should therefore *not* block the merge attempt, leaving the review requirement as the isolated blocking cause, exactly as the experiment wants. But if the merge attempt instead reports a **signature** failure rather than a review failure, that is a *third* bucket — neither an orchestration bug nor a disproof of `autoMergeBlockedByReviewRequirement` — and must be recorded as such per AC6, not conflated.

## 6. Handoff guidance for QA → DevOps

**QA:** the diff is one hunk, two lines, `docs/CONTRIBUTING.md` only; verify with `git diff main...HEAD -- . ':(exclude).workflow'`. The factual claim in the new sentence is checkable against the ruleset GET in AC1. Keep your verdict line exactly `**Verdict:** signoff` — the prior run (`e82c5ab6`) died at `condition_blocked(transition=qa-to-devops): verdictResolved=<unparseable>`, so a malformed verdict is the single highest-probability way this 6th dispatch fails the same way the 5th did.

**DevOps — the crux turn.** Capture, as live evidence not inference:
1. PR number + URL on `api`.
2. Re-run the ruleset GET at merge-attempt time (field to quote: `required_approving_review_count`, `bypass_actors`, `current_user_can_bypass`).
3. PR `mergeable_state` + `reviewDecision` at the moment auto-merge is attempted (expect `REVIEW_REQUIRED` / blocked, 0 of 2).
4. Whether `autoMergeBlockedByReviewRequirement` fired, and whether the run ends as a **plain open PR** (title *without* an `[escalated]` prefix) vs. an escalation.
5. Then update `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` in the **`forge` repo** (a different repo — separate commit/PR). Flip `status:` → `shipped` **only if** AC3–AC5 all hold cleanly. If blocked again, say so plainly with the specific blocker; do not re-assert `shipped` without new evidence.

Do **not** touch PR #450 (prior run's open draft) — operator's call.

**Empty-PR-OK:** no — this run carries a real source change (`docs/CONTRIBUTING.md`, +2/-2) outside `.workflow/`, so the auto-merge gate has genuine content to act on, which is precisely what the experiment requires.

**Verdict:** signoff