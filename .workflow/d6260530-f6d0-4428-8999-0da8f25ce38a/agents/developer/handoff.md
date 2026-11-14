# Developer Handoff — `api-release-rerun-fails-existing-github-release`

## 1. Commit + branch identity

- **Commit SHA:** `6c50147661` (full: `6c5014766b186882c6732b0bfe5c9291a61b62c0`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/d6260530-f6d0-4428-8999-0da8f25ce38a`
- **PR:** engine auto-opens at handoff time
- DCO: `Signed-off-by: David Black (developer) <forge-agent+developer@simple-forge.com>` present (`.github/workflows/dco.yml` gate satisfied)
- Working tree clean after commit (`git status --porcelain` → empty)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `scripts/create-github-release.sh` | +44/-5 | Replaced the unconditional `gh release create` with an `if gh release view … ; then gh release upload --clobber ; else gh release create … ; fi` branch. Updated the header's **Exit codes** block (now mentions assets-refreshed) and added an Idempotency + Verification note. Comment block above the branch documents why `upload` takes no `--title`/`--generate-notes`/`--verify-tag`, and why a transient `view` failure falling through to `create` is deliberate (no retry/backoff — per Architect Refinement B). |
| `scripts/test-create-github-release.sh` | +205/-0 (new file, `+x`) | New branch-coverage harness. Stub `gh` on `PATH` records argv and takes its `release view` exit code from `STUB_GH_VIEW_EXIT`; dist bundles fabricated in `mktemp -d` via `SC_DIST_BUNDLE_DIR`. 5 scenarios / 27 checks. Pure bash, no network, no new tooling. |
| `docs/RELEASES.md` | +44/-1 | Qualified the quoted `gh release create` block with "on a tag that has no Release yet"; added a **Re-running a release job** subsection showing the branch, why notes/title aren't regenerated, and an explicit statement that the unsigned-release refusal is *not* relaxed on re-run. Points at the new test script. |
| `.github/workflows/push.yaml` | +10/-0 | Added a `test release scripts` step to the `test` job (before `go test ./...`) running `./scripts/test-create-github-release.sh`. |
| `.github/workflows/branch.yaml` | +7/-0 | Same step in the PR-gate workflow's `test` job, so a regression is caught before merge rather than on the production tag. |

**Scope note (beyond Architect's plan, flagged deliberately):** Architect scoped this to the script + `docs/RELEASES.md`, with AC5 satisfiable by a documented manual procedure. I wrote an executable test instead and wired it into CI. Rationale: a documented manual procedure for the *last step of the production release pipeline* only gets executed when someone is already debugging a red release — and the regression the brief most fears (AC3, the unsigned refusal getting scoped to one branch) is exactly the kind that a manual procedure silently stops covering. The harness needs no new dependency, so PM's reason for CUTting bats (importing tooling) doesn't apply. If the reviewer wants the CI wiring reverted, the two workflow hunks are independent of the fix itself.

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Re-run of a release run whose Release already exists finishes green | PASS | `scripts/create-github-release.sh:140-146` — `gh release view` success → `upload` path. Test scenario 2 (`existing-tag`) asserts `exit 0` with `STUB_GH_VIEW_EXIT=0`. Note `set -e` is still live, so a *genuinely* broken upload still reds the job — correct per AC1's intent. |
| AC2 | Existing assets refreshed via `upload --clobber`, not duplicate-create | PASS | `scripts/create-github-release.sh:142-145`. Scenario 2 asserts `gh release upload <ver>` **and** `--clobber` were called, and `gh release create` was **not** (`expect_gh_not_called`). Also asserts `--generate-notes`/`--verify-tag` are absent — `gh release upload` rejects them, so passing them would break the recovery path for a different reason. |
| AC3 | Unsigned-release refusal unchanged, still fires on a re-run, before create/upload | PASS | Guard untouched at `scripts/create-github-release.sh:69-73` (diff shows no edit in that range), structurally ahead of the new branch. Scenario 3 (`unsigned-fresh`) and scenario 4 (`unsigned-existing`, `STUB_GH_VIEW_EXIT=0`) both assert `exit 1`, the `No SLSA provenance` message, and **zero** `gh` calls. Mutation-verified: gating that guard on `! gh release view …` fails 4 checks (see §4). |
| AC4 | Preview/RC short-circuit unchanged | PASS | Guard untouched at `scripts/create-github-release.sh:54-57`. Scenario 5 loops both spellings (`-pre.abc1234`, `-pre.…-preview.…`) with a **signed** bundle present and `STUB_GH_VIEW_EXIT=0`, so the `exit 0` can only come from the preview guard; asserts the `Skipping Release creation` message and zero `gh` calls. |
| AC5 | Test or clear manual procedure covering both branches (fresh → create; existing → upload --clobber) | PASS | `scripts/test-create-github-release.sh` — executable, covers both branches plus both regression guards (27 checks), wired into `push.yaml:496` and `branch.yaml:370`. Narrative procedure also documented in `docs/RELEASES.md` → "Re-running a release job". |

## 4. Tests run

**Syntax gate (Architect's requested `bash -n` dry-run):**
```
$ bash -n scripts/create-github-release.sh && echo OK
bash -n scripts/create-github-release.sh: OK
$ bash -n scripts/test-create-github-release.sh && echo OK
bash -n scripts/test-create-github-release.sh: OK
```

**Branch-coverage harness:**
```
$ ./scripts/test-create-github-release.sh; echo "rc=$?"
all 27 checks passed
rc=0
```

**Mutation check A — revert the fix to the pre-fix unconditional `create`** (proves the test actually detects the reported bug, rather than passing vacuously):
```
FAIL [fresh-tag] expected a gh call matching 'gh release view 2026.10.99'; calls were:
FAIL [existing-tag] expected a gh call matching 'gh release upload 2026.10.99'; calls were:
FAIL [existing-tag] expected a gh call matching '--clobber'; calls were:
FAIL [existing-tag] did NOT expect a gh call matching 'gh release create'; calls were:
8/27 checks FAILED
```

**Mutation check B — weaken AC3 by gating the unsigned refusal on `! gh release view …`** (the precise regression the brief warns about):
```
FAIL [unsigned-existing] exit status: want 1, got 0
FAIL [unsigned-existing] expected output to contain 'No SLSA provenance'; output was:
FAIL [unsigned-existing] expected zero gh calls; calls were:
4/27 checks FAILED
```
Both mutations were applied in `/tmp` copies and deleted afterwards; the committed tree is unmodified by them (`git status --porcelain` empty).

**Workflow YAML validity + step ordering:**
```
$ python3 -c "...yaml.safe_load...jobs['test']['steps']"
.github/workflows/push.yaml   -> ['Set up Go', 'test release scripts', 'test']
.github/workflows/branch.yaml -> ['Set up Go', 'test release scripts', 'test']
```

**Artifact verification gate** — every claimed path carries *my* commit from *this* run:
```
scripts/create-github-release.sh       6c501476 David Black (developer) | fix(release): make create-github-release.sh idempotent on re-run
scripts/test-create-github-release.sh  6c501476 David Black (developer) | ...
docs/RELEASES.md                       6c501476 David Black (developer) | ...
.github/workflows/push.yaml            6c501476 David Black (developer) | ...
.github/workflows/branch.yaml          6c501476 David Black (developer) | ...
```

**Skipped deliberately:** `make check` / `go build` / `go test` / `make swag` — this change touches zero `.go` files (`git diff --name-only | grep -c '\.go$'` → `0`) and no swagger-annotated handler, so the Go gates have nothing to exercise. Per CLAUDE.md those gates cover the Go module, not `scripts/`. `shellcheck` is not installed on this runner (`which shellcheck` → not found) and is not a CI gate in this repo (`grep -rn shellcheck .github/workflows/ welder.yaml Makefile` → no hits), so it could not be run.

## 5. Notes for QA / downstream

- **Live smoke is not blocked on a credential I lack** — it is blocked on a *real production release*. The fix's existing-tag branch can only be exercised end-to-end by re-running a release job on a tag that already has a Release. The stub-`gh` harness covers the logic; the live confirmation naturally happens on the next re-run (the brief's own repro: api run 39571929882). QA need not chase a secret for this.
- **The one thing worth a second pair of eyes:** whether adding `test release scripts` to `branch.yaml` + `push.yaml` is welcome scope. The fix is correct without it; the two hunks are trivially revertable.
- **Roadmap:** `docs/roadmap/` does not exist in this repo (`ls docs/roadmap` → no such file). The item `api-release-rerun-fails-existing-github-release.md` lives in the `forge` umbrella repo, so flipping it `in-progress` → `shipped` is a **cross-repo** edit. Per the cross-repo guard I did **not** touch it. `cross_repo_dependency`: `simple-container-com/forge` — needs `docs/roadmap/items/api-release-rerun-fails-existing-github-release.md` moved to `status: shipped` with a History line citing commit `6c50147661` in `simple-container-com/api`, plus the matching index row in `docs/roadmap/README.md`. That requires an operator-approved sibling run against `forge`.

**Verdict:** signoff