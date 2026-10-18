Both ruled out conclusively: the embed root is `docs/docs/**` (the edited `docs/CONTRIBUTING.md` sits outside it), and the Go hit is a filename-classification map constant, not file content. Zero Go surface confirmed by inspection, not assumption.

# QA Handoff — `api-repo-branch-protection-blocks-sdlc-automerge` (6th dispatch)

## 0. QA posture

I verified the developer's claims rather than inheriting them. Two upstream statements did **not** survive verification (one materially): the Go toolchain **is** present in my sandbox, so the build gate the developer declared unrunnable was in fact runnable, and I ran it. Details in §5. The diff itself is clean and the factual claim it makes is true against the live ruleset I fetched myself.

## 1. Commit + branch identity

- **Verified commit:** `a1a3990b84055a0b8b823d18345104fccc5c5f44` (short `a1a3990b84`) — `docs(contributing): correct stale review-count to match live branch protection`
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/2e40aa56-b159-48dd-85c3-aa71844f4b08`
- **Verification gate run:** `git log -1 --format="%H %an %s" -- docs/CONTRIBUTING.md` → `a1a3990b84055a0b8b823d18345104fccc5c5f44 David Black (developer) docs(contributing): correct stale review-count…` — a real commit by **this run's** developer on the real source path, not the engine's init commit (`cd04c7e0`) and not a prior run's.
- **Substance gate:** PASSED — the non-`.workflow` diff is a real source file. `main` == `origin/main` == `0a008cdeb9` (so the diff base is current, not stale).
- **No QA mutations** — `git status --porcelain` empty. I did not widen the diff.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files touched in this QA turn (verification-only; tree left clean) |

Developer's diff, which I verified rather than restated — `git diff main...HEAD --numstat -- . ':(exclude).workflow'` → exactly `2	2	docs/CONTRIBUTING.md`, single hunk `@@ -110,8 +110,8 @@`:

```diff
-- Code review is **required** — current branch protection enforces 1
-  reviewer minimum; this will increase to 2 once admin-UI gates land.
+- Code review is **required** — branch protection enforces 2 approving
+  reviews plus code-owner review before merge; nothing self-merges here.
```

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Ruleset `9144757` active, `required_approving_review_count: 2` | **PASS** | **My own** `GET /repos/simple-container-com/api/rulesets/9144757` → **200**, `Date: Sun, 18 Oct 2026 00:45:24 GMT`, ETag `W/"bca82774d9f0…"`. `enforcement:"active"`; `pull_request` → `required_approving_review_count:2`, `require_code_owner_review:true`, `require_last_push_approval:true`, `dismiss_stale_reviews_on_push:true`, `required_review_thread_resolution:true`, `allowed_merge_methods:["squash"]`; plus `required_status_checks` (7 contexts, `strict…policy:true`), `required_signatures`; `bypass_actors:[]`, `current_user_can_bypass:"never"`; `updated_at:2026-06-30T19:07:03Z`. Distinct ETag from developer's ⇒ independent fetch, not a quoted value. |
| AC2 | Change genuinely trivial (docs-only), lands on this run's branch | **PASS** | Commit `a1a3990b84`, `2 2 docs/CONTRIBUTING.md`, one hunk. Every changed non-`.workflow` path enumerated → `docs/CONTRIBUTING.md -> ext=md`; grep for `\.(go\|mod\|sum\|ya?ml)$` → **NONE**. Content accuracy independently checked: the new sentence's "2 approving reviews plus code-owner review" matches AC1's live JSON exactly. |
| AC3 | PR opens against `api:main` from this run's branch | **FAIL-blocked** (downstream, correctly) | `list_pull_requests(head=forge/workflow/…/run/2e40aa56-…, state=all)` → **no PR for this branch** as of now. Engine opens it at handoff; DevOps owns recording the number. Not a defect — expected ordering. |
| AC4 | `autoMergeWithBackoff` attempts merge, cannot satisfy review gate (0/2) | **FAIL-blocked** (downstream) | Not observable from a QA turn — needs the orchestrator's merge attempt. Structurally pre-determined: `bypass_actors:[]` + `current_user_can_bypass:"never"` (AC1) ⇒ no App token can bypass, so 0/2 is guaranteed at merge time. |
| AC5 | `autoMergeBlockedByReviewRequirement` fires; clean open PR, not escalation | **FAIL-blocked** (downstream — the crux) | DevOps-owned. This is the observation 5 prior dispatches never reached. My job was to not be the reason it fails again (see AC6). |
| AC6 | Orchestration-layer bug distinguished from a fix defect | **PASS** (for my turn) | Nothing orchestration-level broke here. I read the run spec: `qa-to-devops` condition is `handoff.verdict == 'signoff'` — the exact gate that failed run `e82c5ab6` with `verdictResolved=<unparseable>`. My verdict line is the required markdown-bold form. The three issues I *did* hit are **environment/resource** limits, explicitly **not** defects in the branch-protection fix — bucketed in §5. |
| AC7 | Roadmap item `## Status`/`## History` updated in `forge` repo | **FAIL-blocked** (not mine, and correctly so) | Outcome (AC3–AC5) is still unobserved; writing it now would fabricate the result this run exists to measure. Owned by whoever reaches furthest — DevOps. |
| Gate A/B | Isolated-stack deploy + live smoke | **N/A — not requested** | Keyword scan of the brief proper (excluding the quoted triggering-run block) for `gate a\|gate b\|isolated\|deploy\|smoke\|simple-forge.com` → only two incidental hits, both narrative (PR #583 "deployed"; the DoD's "live `api`-targeted SDLC dispatch"). Neither requests an isolated-stack deploy. Structurally N/A too: `deploy-isolated-stack.yml` stands up forge services, and this run's repo is the `sc` CLI module. |
| Lint | Static-lint gate | **FAIL-blocked (tooling absent) — risk nil** | `.golangci.yml` exists and `Makefile:102` has `lint:` → `$(GOLANGCI) run --fast --timeout 5m` with `GOLANGCI ?= $(CURDIR)/bin/golangci-lint`. But `bin/` is **absent** and `golangci-lint` is not on PATH (needs `welder run tools`). Not run. Risk is nil and provable: lint only inspects Go, and the diff has zero Go surface (AC2). CI's required `Build Setup (clean, tools, schemas, lint, fmt)` check runs it on the PR regardless. |

## 4. Tests run

**`make check` — RAN, did NOT pass: terminated under resource pressure (reported as a non-pass, not massaged into a PASS).**

```
### make check (go vet ./...) — capped per CLAUDE.md ###
GOMAXPROCS=1 GOMEMLIMIT=6GiB go vet -p 1 ./...
make: *** [Makefile:87: check] Terminated
```

Honest reading: it ran ~25 min on a **cold** cache and my timeout killed it. `MemAvailable` fell to **0.2 GB** mid-run against CLAUDE.md's 2.3 GB peak for this target — this sandbox has `MemTotal: 3.7 GB`, well under the 12 GB CLAUDE.md assumes. Per CLAUDE.md's explicit instruction I did **not** retry the same command. The trailing `exit=0` in my own output is `tail`'s status, not make's — calling that a pass would be exactly the misread the build-gate rule exists to prevent.

**Scoped `go vet` — PASSED, proving the toolchain works and the failure was resource-bound, not code-bound:**

```
### scoped vet: a light, dependency-free pkg ###
GOMAXPROCS=1 GOMEMLIMIT=2GiB go vet -p 1 ./pkg/util/...
### scoped vet rc=0 ###
```

**Why the un-finished full vet does not gate this diff — verified by inspection, not assumed.** I chased the two ways a markdown file *could* reach the Go build and closed both:

1. `//go:embed docs` in `docs/embedfs.go` — the directive is in package `docs`, so its root is **`docs/docs/**`**. `docs/docs/` exists (confirmed); the edited `docs/CONTRIBUTING.md` sits *outside* the embedded tree. Not compiled in.
2. `pkg/assistant/analysis/file_analysis.go:159` contains `"CONTRIBUTING.md": true` — a **filename-classification map key**, not file content. Editing the file's prose cannot affect it.

Plus: no `.go`/`go.mod`/`go.sum`/`yaml` in the diff, so no `make swag` drift and no `_test.go` surface. SPA build N/A — no `internal/ui` in this repo.

## 5. Findings — upstream claims that failed verification

**(a) MATERIAL — the developer's "Go toolchain absent" claim is false for this environment.** The developer skipped `make check` citing `go: command not found`. In my sandbox:

```
$ command -v go  → /usr/local/go/bin/go
$ go version     → go version go1.26.9 linux/amd64
```

Character sandboxes are provisioned independently — Go present here, absent there; developer reported `MemTotal: 11.7 GB`, I have `3.7 GB`. **Consequence:** the build gate was runnable and I ran it (§4) rather than inheriting a skip. I'm flagging this as a **process defect, not a content defect** — the diff is still sound. Recorded to memory so future QA turns re-probe instead of trusting an upstream "toolchain absent".

**(b) CONFIRMED — `docs/ARCHITECTURE.md:79` understates the control.** Developer's finding verified verbatim:

```
./docs/ARCHITECTURE.md:79: | **Dev → GH Actions** | … | Branch protection (signed commits required, ≥1 reviewer, DCO trailer enforced) |
```

In the **Trust boundaries** table. "≥1" is not strictly false when 2 are required, but understating a control in a security document is a real (minor) doc defect. I **endorse not fixing it here** — PM and Architect both set a hard single-hunk scope guard, and widening the diff would compromise the merge-path observation. **Recommend filing as a separate trivial docs item**; it's also a ready-made payload for a future `api` dispatch.

**(c) CONFIRMED — unsigned commit, and I agree it does not confound the experiment.** `gpg: NOT INSTALLED`, `ssh-keygen: NOT INSTALLED` — signing is impossible here. Commit `a1a3990b84` carries a correct DCO trailer with an exact author match (`Signed-off-by: David Black (developer) <forge-agent+developer@simple-forge.com>`, author identical ⇒ `dco.yml` satisfied), and `%GS` (signer) is empty ⇒ unsigned. The developer's reasoning holds: `allowed_merge_methods:["squash"]` means GitHub re-signs the squash commit server-side with its own key, satisfying `required_signatures` at merge time.

**Carry-forward for DevOps (third bucket, per AC6):** if the merge attempt reports a **signature** failure rather than a **review** failure, that is neither an orchestration bug nor a disproof of `autoMergeBlockedByReviewRequirement` — record it as its own bucket.

## 6. Handoff guidance for DevOps — the crux turn

Capture as **live evidence, not inference** (future-tense = auto-fail):

1. PR number + URL on `api` (none exists for this branch yet — I checked).
2. Re-run the ruleset GET at merge-attempt time; quote `required_approving_review_count`, `bypass_actors`, `current_user_can_bypass`.
3. PR `mergeable_state` + `reviewDecision` at the attempt (expect blocked / `REVIEW_REQUIRED`, 0 of 2).
4. Whether `autoMergeBlockedByReviewRequirement` **fired**, and whether the run ends as a **plain open PR** (title *without* an `[escalated]` prefix) vs. an escalation.
5. Then update `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` in the **`forge` repo** (different repo, separate commit/PR). Flip `status:` → `shipped` **only if** AC3–AC5 all hold cleanly. If blocked again, say so plainly — do not re-assert `shipped`/`code-complete` without new evidence. Please also record finding (a) and (b) above.

Do **not** touch PR #450 (prior run `e82c5ab6`'s open draft, still `[escalated]`/unmerged — confirmed via `list_pull_requests`) — operator's call.

## 7. Verdict

The diff is real, minimal, single-hunk, docs-only, factually correct against the live ruleset, DCO-compliant, and committed by this run's developer on the real source path. The substance gate passes on positive evidence (real SHA + real source path + verification commands pasted). No auto-reject trigger is present: there are no "zero code changes" / "no actual code" / fabrication findings, and the source-file row count is 1, not 0. The build gate's non-completion is a documented resource limit on a zero-Go-surface diff, mitigated by a passing scoped vet and by closing both embed/reference paths by inspection — not a code failure, and I've reported it as a non-pass rather than dressing it as green.

AC3–AC5 are FAIL-blocked on DevOps by design — those are the downstream observations this dispatch exists to make, and the correct action is to advance the chain so DevOps can finally run, not to fail-close here. This is the one case where handing off is right: DevOps holds the merge-path observation I structurally cannot make.

**Empty-PR-OK:** no — this run carries a real source change (`docs/CONTRIBUTING.md`, +2/-2) outside `.workflow/`, which is exactly what the auto-merge gate needs to act on.

**Verdict:** signoff