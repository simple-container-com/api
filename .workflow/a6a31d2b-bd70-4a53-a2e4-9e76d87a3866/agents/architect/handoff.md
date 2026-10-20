[thinking] # Architect Handoff — `api-release-binary-roundtrip` verification

## 0. Role framing

This is a verification-only slice; as PM, my predecessor already did the core investigative work and reached a clear, correctly-evidenced finding. My job as Architect is to (a) independently re-verify the load-bearing claims rather than rubber-stamp them, (b) give a structural/architectural read on the fix itself (is `ci/concurrency-release-fleet-slots` the right shape, any coupling/regression risk), and (c) give unambiguous downstream routing. I did **not** take the PM's word for it — every claim below was re-checked in this turn against live repo state.

## 1. Commit + branch identity

No code mutations in this turn — Architect role, verification/design review only. Workflow-run branch per context: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/a6a31d2b-bd70-4a53-a2e4-9e76d87a3866` (not used — nothing written to it).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | No file mutation this turn |

## 3. Independent re-verification (2026-10-20, this turn)

I re-ran the PM's checks myself rather than trusting the prior handoff:

| Check | My result (this turn) | Source |
|---|---|---|
| `api` PR #461 state | `state: open` — confirmed still open, still the top row in `list_pull_requests(state=open)` for `api` | `list_pull_requests` call, this turn |
| `main` HEAD | `d785db6ffd8fd35f70d7b86bc50a50ed04e80cb0` ("ci: install the pulumi CLI...", merged PR #460) | `list_repository_commits(ref=main)`, this turn |
| `github-actions.Dockerfile` @ `main` | **No `gobuild` stage.** Final stage does `COPY dist/github-actions ./github-actions` — i.e. still artifact-fed, pre-fix shape | `read_repository_file(ref=main)`, this turn — full file read |
| `github-actions.Dockerfile` @ `ci/concurrency-release-fleet-slots` (PR #461 head) | **Has** `FROM golang:1.26-alpine@sha256:3082400e...AS gobuild`, `ARG VERSION=0.0.0`, `-ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}"`, `COPY --from=gobuild /out/github-actions ./github-actions` | `read_repository_file(ref=ci/concurrency-release-fleet-slots)`, this turn — full file read |
| `push.yaml` @ `main` still has `download github-actions binary` | Confirmed via code search — hit is on the `main`-ref blob | `search_repository_code`, this turn |
| Roadmap item location | `docs/roadmap/items/api-release-binary-roundtrip.md` **does not exist in `api`** (file-not-found) — it lives in the **`forge`** repo | `read_repository_file` 404 in `api`; successful read in `forge`, this turn |

**Conclusion: the PM's finding stands, independently confirmed.** PR #461 is unmerged; `main` has no in-image `gobuild` stage; `push.yaml` on `main` still carries the binary-artifact download steps. There is no post-#461 release run to inspect because #461 has not merged.

## 4. Architectural review of the fix itself (PR #461, on its branch)

Reviewing the *design*, not just checking it merged — since I'm going to recommend it get merged, it should be sound:

- **The in-image `gobuild` stage is the right shape for this constraint.** Build-arg plumbing (`ARG VERSION=0.0.0` → `-X=…build.Version=${VERSION}`) matches the old `build-binaries` flags exactly (`-trimpath -buildvcs=false`, `CGO_ENABLED=0`), so this isn't a silent behavior change — it's the same binary, built in a different place. Good: no new drift surface between the two build sites.
- **The `.dockerignore` allowlist is correctly structured** as deny-by-default (`*`) with explicit un-ignores (`!cmd !pkg !internal !go.mod !go.sum !tools.go !bin`) — `.sc/`, `docs/`, `.git/` are excluded by construction, not by a denylist that can rot. This is the right pattern for a security-sensitive build context (verified on PR branch by direct read).
- **One real regression risk, and the author already flagged + mitigated it**: the Dockerfile's own comment block ("BLACKSMITH TRADEOFF") says explicitly — if CI ever moves off the self-hosted fleet back onto ephemeral hosted runners (Blacksmith), the `type=cache` mounts this stage depends on for incremental compiles don't survive an ephemeral runner, and the arithmetic that justified this fix could invert. The author left a numbered revert recipe in the file. This is the correct way to carry an architectural trade-off — I have no pushback here, it's exactly the kind of "measure before keeping" discipline I'd ask for.
- **Memory risk called out in the PR body is real and gates mergeability, not just my verification task**: `github-actions` shares 2092/2095 packages with `./cmd/sc` (peak 12.9 GB) on a 16 GB runner; PR #461's own `Build github-actions` CI check is the live probe for this. I did not independently re-run that CI job — it's out of scope for an architecture review and the PR's CI state is the operator's/reviewer's signal to check before merge, not mine to fabricate.
- **Scope concern (same as PM flagged, I agree)**: #461 bundles the binary-round-trip fix with four other concerns (concurrency supersede, release-integrity gates, hosted-runner re-routing for 7 jobs). From a change-management standpoint I'd normally push back on bundling, but the PR body states this was **consolidated at the operator's explicit direction** ("fix right away in the same PR we're waiting", and PR description: "this PR now carries what was #461–#465 ... each self-contained and independently revertable") — so this isn't a drive-by scope creep, it's a deliberate operator call with revert-ability preserved per-commit. I won't re-litigate that decision; noting it for the record only.

**Architectural verdict on the fix: sound, no design changes needed.** The blocker is entirely procedural (unmerged PR, human-review branch protection), not technical.

## 5. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Find first `api` release run after `api#461`/`cf7e9104` merged to `main` | **FAIL-blocked** | PR #461 `state=open` (re-checked this turn, `list_pull_requests`); `main` HEAD still `d785db6` — no merge has occurred |
| AC2 | `download …binary` steps + `dist/*` upload structurally gone from workflow | **FAIL-blocked** | `push.yaml` @ `main` still contains `download github-actions binary` step (code-search hit on main-ref blob, this turn) |
| AC3 | Both images build binary via in-image `gobuild` stage | **FAIL on `main` / confirmed-present on PR branch** | `main`: `COPY dist/github-actions ./github-actions` (no gobuild stage) — full-file read, this turn. PR branch: `FROM golang:1.26-alpine@sha256:3082400e...AS gobuild` + `COPY --from=gobuild /out/github-actions` — full-file read, this turn |
| AC4 | Binary reports correct `-X build.Version=${VERSION}` not `0.0.0` default | **Not verifiable — no merged commit, no built artifact to inspect** | PR branch Dockerfile ldflags inspected: `-X=github.com/simple-container-com/api/internal/build.Version=${VERSION}` with `ARG VERSION=0.0.0` fallback — plumbing looks correct by static read, but never exercised by a real release build |
| AC5 | `.dockerignore` excludes `.sc/`, `docs/`, `.git/` from build context | **FAIL on `main` (file absent) / PASS on PR branch content** | `.dockerignore` does not exist on `main`; on PR branch it's an allowlist (`*` deny + explicit `!cmd !pkg !internal !go.mod !go.sum !tools.go !bin`) which structurally excludes `.sc`/`docs`/`.git` |
| AC6 | Fix any regression found live | **N/A** | No live run exists to regress-test; the one open risk (OOM probe on `Build github-actions`) is the PR's own CI, not something this verification turn can observe or fix |

## 6. Downstream routing — do this, not a re-chase

1. **Do not dispatch another "find the release run" attempt.** Two independent passes (PM's, mine) confirm it doesn't exist yet. Further chasing burns compute against a wall that only an operator action removes.
2. **Operator action required, named precisely**: merge (or otherwise land) `api` PR #461 (`ci/concurrency-release-fleet-slots`, head `0a135681`) to `main`. This repo enforces human-reviewed merges (branch protection — see the separately-tracked `api-repo-branch-protection-blocks-sdlc-automerge` item, PRs #450/#454 still open); SDLC automation structurally cannot do this itself.
3. **Roadmap item location correction, structural note**: `docs/roadmap/items/api-release-binary-roundtrip.md` lives in the **`forge`** repo, not `api` — confirmed by direct read this turn (404 in `api`, success in `forge`). This workflow run's branch is scoped to the **`api`** repo only. I cannot write a correct roadmap update from here without either (a) writing to the wrong repo's default branch (forbidden — "write to your run's branch, not default branch"), or (b) writing to a branch name that may not exist in `forge`. **Whoever picks this up next must either**: have `forge`-repo write access on an appropriately scoped branch, or this correction should route back through the Roadmap Driver (which already owns `docs/roadmap/` edits per its own prior handoff in this chain) rather than through this `api`-scoped SDLC run.
4. **Recommended roadmap correction content** (for whichever character/run has the right repo scope): flip `status: in-progress` → `status: code-complete` (not `shipped`), set `blocked_on: "operator: merge api PR #461 (ci/concurrency-release-fleet-slots, head 0a135681) — requires human review per branch protection; contains the binary-round-trip fix plus 4 bundled, independently-revertible commits"`, update `next:` to point at re-verification against `main` post-merge, append a `## History` line dated 2026-10-20 recording this verification attempt's outcome (both PM and Architect turns reached the same finding).
5. **When #461 merges**, the re-verification must run against the **actual merge commit on `main`**, not the PR branch: confirm `push.yaml`'s download steps are gone on `main`, find the next `push.yaml` run on `main`, pull the `gobuild` stage's build log and the image's baked-in version (e.g. exec into the pushed image and check the version string, or `strings` the binary) to confirm `VERSION` flowed through and isn't `0.0.0`.

## 7. Tests run

N/A — no code changed this turn. Verification method was direct repo-state reads (`list_repository_commits`, `read_repository_file`, `search_repository_code`, `list_pull_requests`) against both `main` and the PR branch, cross-checked against the PM's prior-turn findings rather than trusted blind.

**Empty-PR-OK:** yes — this turn is pure verification/architecture review (repo-state reads only); the finding (PR #461 still unmerged, same as PM's turn) is the deliverable, independently re-confirmed rather than carried forward unchecked.

**Verdict:** signoff
**Provisional:** no — repo was reachable; all file paths and commit references above were read directly in this turn, not guessed.