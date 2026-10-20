[thinking] # PM Handoff — `api-release-binary-roundtrip` verification (Main Forge SDLC)

## 1. Restated requirement

The brief asks for **live verification only** (code fix is claimed already merged) that `api#461` / commit `cf7e9104` killed the ~15-min binary artifact round-trip in `push.yaml`'s `docker-build` job — by finding the first post-merge release run and confirming:

1. `download cloud-helpers binary` / `download github-actions binary` steps and the `dist/*` upload from `build-binaries` are structurally gone.
2. Both `github-actions` and `cloud-helpers-aws` images build their binary via an in-image `gobuild` stage.
3. The baked binary reports the correct `-X … build.Version=${VERSION}` (not the Dockerfile's `0.0.0` default).
4. `.dockerignore` actually excludes `.sc/`, `docs/`, `.git/` from the build context.

## 2. Critical finding — the brief's premise does not hold

**The fix is NOT merged.** I verified this directly, not from the brief's claim:

| Check | Result |
|---|---|
| `main` HEAD | `d785db6` (PR #460, "install pulumi CLI") — **no gobuild stage** |
| `github-actions.Dockerfile` @ `main` | Old shape: `COPY dist/github-actions ./github-actions` (artifact-fed), confirmed by direct file read |
| `github-actions.Dockerfile` @ `ci/concurrency-release-fleet-slots` (PR #461 head) | **Has** the `gobuild` stage, cache mounts, `COPY --from=gobuild /out/github-actions` — confirmed by direct file read |
| PR #461 state | `"state":"open","merged_at":null` — commit `cf7e9104` is one of **5 commits** now on this branch (HEAD is `0a135681`, last pushed 2026-10-19T20:22Z, after cf7e9104) |
| `push.yaml` @ `main` | **Still contains** `download github-actions binary` / `download cloud-helpers binary` steps (confirmed via code search — hits only on `main`-ref blob) |
| `.dockerignore` | Does not exist on `main` at all; only exists on the PR branch |

So: **there is no post-merge release to inspect** because nothing has merged. The brief's "## What already landed" section is describing work that is still sitting in an **open PR** (#461), not a shipped commit on `main`. The roadmap item's own `next:` field was written under the same incorrect assumption.

## 3. Why this matters structurally (not just a timing gap)

This repo requires human-reviewed merges — `CLAUDE.md`: *"Branch protection also requires human review — nothing self-merges here."* The org already has a **known, separately-tracked roadmap item** for exactly this class of problem: `api-repo-branch-protection-blocks-sdlc-automerge` (escalated, PRs #450/#454 still open against it). SDLC automation cannot itself merge `api` PRs. That means:

- This verification slice **cannot complete** until a human operator merges PR #461 (or at minimum lands `cf7e9104`'s content on `main`).
- PR #461 as it stands bundles **far more** than the binary-round-trip fix — concurrency/supersede logic, release-integrity gates (`fail-fast: false`, `if-no-files-found: error`, sidecar-assertion fix), and re-routing 7 jobs to hosted runners. That's a 405-line, 11-file, multi-concern PR awaiting review — not a trivial rubber-stamp.
- The PR's own body flags an unresolved risk requiring live proof: the "memory OOM probe" on `Build github-actions` (shares 2092/2095 packages with `./cmd/sc`, peak 12.9GB on a 16GB runner) — this needs to pass CI before anyone should merge it.

## 4. Scope decisions

| Item | Status |
|---|---|
| Confirm gobuild stage exists & builds correctly in both Dockerfiles | MUST — already proven structurally via file diff (PR branch), just not live yet |
| Confirm `.dockerignore` allowlist excludes `.sc/`/`docs/`/`.git/` | MUST — content read and confirmed correct on PR branch (`* ` then `!cmd !pkg !internal !go.mod !go.sum !tools.go !bin` — nothing resurrects `.sc`, `docs`, or `.git`) |
| Find and inspect a **real post-merge** release run | MUST, but **BLOCKED** — no such run exists; PR unmerged |
| Fix any regression found live | CUT for this turn — nothing to regress-test against yet |
| Update roadmap item to `shipped` | CUT — explicitly forbidden by the brief's own rule when blocked: "stop at code-complete... do NOT mark shipped" |
| Flag the operator-only blocker precisely | MUST |

## 5. Acceptance criteria

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Find first `api` release run after `api#461`/`cf7e9104` merged to `main` | **FAIL-blocked** | PR #461 `state=open, merged_at=null` (GitHub API, checked 2026-10-20); `main` HEAD is still `d785db6` (pre-fix) |
| AC2 | `download …binary` steps + `dist/*` upload structurally gone from the workflow | **FAIL-blocked (not yet true on `main`)** | `push.yaml` @ `main` still has `download github-actions binary` step (code-search hit on main-ref blob); only the PR branch (`ci/concurrency-release-fleet-slots`) has them removed |
| AC3 | Both images build binary via new in-image `gobuild` stage | **PASS on PR branch, FAIL on `main`** | `github-actions.Dockerfile` @ PR branch has `FROM golang:1.26-alpine@sha256:3082400e… AS gobuild` + `COPY --from=gobuild /out/github-actions`; @ `main` still `COPY dist/github-actions` |
| AC4 | Binary reports correct `-X … build.Version=${VERSION}` (not `0.0.0` default) | **Not verifiable yet** — no built artifact exists from a merged commit; PR branch Dockerfile shows `ARG VERSION=0.0.0` with `-ldflags "-s -w -X=…build.Version=${VERSION}"`, build-arg plumbing looks correct by inspection but has not been exercised by a real release |
| AC5 | `.dockerignore` excludes `.sc/`, `docs/`, `.git/` from build context | **PASS on PR branch content, FAIL on `main`** (file doesn't exist on `main` at all) | Read `.dockerignore` @ PR branch: allowlist is `cmd pkg internal go.mod go.sum tools.go bin` only — `.sc`, `docs`, `.git` are excluded by the default-deny `*` |
| AC6 | Regression fix + note if anything broke | N/A | No live run to regress-test; the PR's own open risk is the `Build github-actions` OOM probe (noted in PR body, not yet confirmed green) |

## 6. Handoff guidance for Architect / downstream

1. **Do not attempt to "find the release run" again** — it doesn't exist. The correct action is to **stop the verification chase** and route this back as an operator decision: *"PR #461 needs human review + merge before this slice can be verified."*
2. **Roadmap item correction needed** (file mutation — hand to whichever downstream character writes docs, e.g. QA/DevOps per this workflow's convention): `docs/roadmap/items/api-release-binary-roundtrip.md` should move from `status: in-progress` back to `status: code-complete`, with `blocked_on: "operator: merge api PR #461 (ci/concurrency-release-fleet-slots) — contains the binary-round-trip fix (cf7e9104) plus 4 follow-on commits, still open, requires human review per branch protection"`. Do **not** flip to `shipped`.
3. If the operator merges PR #461 before this chain's next turn, re-run the verification against the **actual merge commit on `main`**, not the PR branch — confirm `push.yaml`'s `download …binary` steps are gone on `main` itself, then find the next `push.yaml` run on `main` and pull its job logs for the `gobuild` stage output and the `Promote release tags` step to confirm the correct version string landed in the pushed image (e.g. `docker run --entrypoint sh <image> -c '/root/github-actions --version'` equivalent, or `strings` the binary for the version string).
4. Flag to operator separately (not blocking this handoff): PR #461 is scope-heavy for a "kill the binary round-trip" ask — it also touches concurrency groups, release-integrity gates, and hosted-runner routing for 7 jobs. Confirm with the operator whether they want it merged as one unit or whether a narrower PR carrying just `cf7e9104`'s diff should be cut for faster review.

## 7. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | No code mutations in this turn — PM role, research/verification only |

**Empty-PR-OK:** yes — this turn is pure research/verification (GitHub API + file reads across `main` and the open PR branch); the finding itself (fix unmerged) is the deliverable, and no roadmap/doc edit was made in this turn so downstream can make the correction with full context.

**Verdict:** signoff