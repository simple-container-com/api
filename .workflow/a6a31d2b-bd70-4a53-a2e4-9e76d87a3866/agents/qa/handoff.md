Verification complete. I did not inherit the prior turns' conclusions — I re-ran every load-bearing check myself, and found three things that change the recommendation the chain was about to hand the operator.

# QA Handoff — `api-release-binary-roundtrip` live verification

## 0. Gate decision up front

This slice's central deliverable is **live evidence from a real post-merge release run**. That run **does not exist**, and cannot be made to exist by me: the fix is in an open PR whose own CI is red. Three prior characters each emitted `signoff` on a run that has **zero source changes, zero roadmap changes, and zero delivered items from the brief's Definition of Done**. With `mergeStrategy=auto-merge`, a fourth `signoff` would merge a `.workflow/`-metadata-only no-op PR and signal "done" on a slice that shipped nothing. I'm closing the gate instead.

## 1. Commit + branch identity

**No code mutations in this turn.** Verification-only; there was no regression of mine to fix, and the single owed artifact (the roadmap item) is in a different repository (§5).

- Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/a6a31d2b-bd70-4a53-a2e4-9e76d87a3866`
- HEAD: `776af744c991c955973e847a14183062f745393d` ("forge: character developer (David Black) phase run" — engine commit, not source)
- `git status --porcelain` → empty
- `git diff --name-only origin/main...HEAD -- . ':(exclude).workflow'` → **empty** (entire branch diff is engine metadata)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| *(none)* | — | No file mutations. Verification-only slice; roadmap target lives in `simple-container-com/forge`, outside this `api`-scoped run |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Find first `api` release run after `cf7e9104` merged to `main` | **FAIL — blocked (operator), no such run exists** | `git merge-base --is-ancestor cf7e9104 origin/main` → exit 1 (**not** an ancestor). `main` HEAD = `d785db6ffd8fd35f70d7b86bc50a50ed04e80cb0` (#460). PR #461 `state: OPEN, mergedAt: null`. I enumerated the **20 most recent runs on `main` across every workflow and trigger** — all 20 sit at `d785db6ff`; newest `push.yaml` on main is `39253348066` @ `d785db6ff` (2026-10-19T12:36Z, `cancelled`) — the **pre-fix** run the operator was watching. `cf7e9104` is dated 2026-10-19 20:02:29 +0300, i.e. **after** it. |
| AC2 | `download …binary` steps + `dist/*` upload structurally gone | **PASS on the fix ref / FAIL on `main`** | `main` `push.yaml`: `download github-actions binary` (L516) + `download cloud-helpers binary` (L522), each `path: dist` (L521/527). Fix ref `0a135681`: **zero** matches for `download .*binary` **and zero** for `path: dist`; `download-artifact@` uses drop **5 → 3** (counted both refs). `build-binaries` on fix ref has **no** `upload-artifact` step — steps are only `Set up Go`/`create build directories`/`build ${{ matrix.target }}` → compile-only gate, as the brief describes. |
| AC3 | Both images build via in-image `gobuild` stage | **PASS by static read only — NEVER EXECUTED anywhere** | Fix ref: `github-actions.Dockerfile:129` and `cloud-helpers.aws.Dockerfile:39` both `FROM golang:1.26-alpine@sha256:3082400e…AS gobuild`; `COPY --from=gobuild /out/github-actions` (L169) and `/out/cloud-helpers` (L87). `main` still `COPY dist/github-actions`. **But see Finding 3 — this stage has never run in any CI job, on any ref.** Scanned all 5 Dockerfiles: only these two carry `gobuild`. |
| AC4 | Binary reports correct version, not the `0.0.0` default | **PASS on plumbing + mechanism; unproven in a real image** | Brief's guessed symbol `main.version` **does not exist** (`grep` across `cmd/github-actions/`, `cmd/cloud-helpers/` → no hits). Real symbol: `internal/build.Version`, default `"latest"` (`internal/build/build_info.go:6`). Dockerfiles use `-ldflags "-s -w -X=github.com/simple-container-com/api/internal/build.Version=${VERSION}"` with `ARG VERSION=0.0.0`. `push.yaml` @ fix ref **does** pass it: `build-args: \| VERSION=${{ needs.prepare.outputs.version }}` (L615-616). My own probe: stamped → `2026.10.20-probe`; unstamped → `latest`. Mechanism works; **no built image exists to inspect**. |
| AC5 | `.dockerignore` excludes `.sc/`, `docs/`, `.git/` | **PASS on the fix ref / FAIL on `main`** | `git cat-file -e origin/main:.dockerignore` → `does not exist in 'origin/main'`. Fix ref: deny-by-default `*` then `!cmd !pkg !internal !go.mod !go.sum !tools.go !bin` — nothing resurrects `.sc`, `docs`, or `.git`, so all three are excluded by construction. Correct pattern (allowlist, not a rottable denylist). |
| AC6 | Fix anything regressed; note findings | **PASS — nothing regressed; 3 findings, see §4** | No regression attributable to `cf7e9104`; CI-red root cause independently confirmed as pre-existing (§4 Finding 1). |
| DoD-a | Live evidence from a real post-#461 release | **FAIL — blocked (operator)** | Same as AC1. No run exists. |
| DoD-b | Roadmap item flipped to `shipped` + README `✅ SHIPPED` | **Correctly NOT done** | Brief's own rule: blocked → "do NOT mark shipped". Also structurally impossible here — see §5. |

## 4. Findings (3 new this turn)

### Finding 1 — PR #461's CI is red; I confirmed the root cause from the raw log

`gh run view --log-failed` returned empty, so I refetched via `gh api .../jobs/118339206455/logs` (retry, per transient policy):

```
22:21:02 bin/golangci-lint run --fix --timeout 3m -v
22:24:09 level=info  msg="[loader] Go packages loading at mode 8767 ... took 3m6.269512763s"
22:24:09 level=error msg="Running error: context loading failed: ... context deadline exceeded"
22:24:09 level=error msg="Timeout exceeded: try increasing it by passing --timeout option"
22:24:14 ##[error]Process completed with exit code 4.
```

Failing step is **`fmt` (step 12)** of `Build Setup (clean, tools, schemas, lint, fmt)`, runner `pc-build-5-97a0d3fc`. Package loading alone (3m6.27s) exceeds the 3m budget — it dies before linting a single file. Also `Measure statement coverage` → fail (1h1m37s), `Finalize build in branch` → fail (cascade).

**Not a regression from `cf7e9104`**, verified two ways: `--timeout 3m` is byte-identical on `main` and the fix ref (`welder.yaml:97` both); and `cf7e9104` touches only `.dockerignore`, `push.yaml`, and the two Dockerfiles (`git show --stat` — 4 files, no `.go`, no `welder.yaml`, no `branch.yaml`).

### Finding 2 — `docker-build` still blocks on `build-binaries`, so the wall-clock saving is smaller than the brief implies (new)

Nobody checked the job graph. On **both** refs:

```
needs: [prepare, build-setup, build-platforms, build-binaries, test, build-docs]
```

The 448s×2 *fetch* is genuinely gone, but `docker-build` **still waits for `build-binaries` to finish compiling** — the job was kept as a compile-only gate and left in `needs:`. So the release does not recover the full ~15 min; it recovers the transfer time while still serialising behind the same compile. The fix ref partly compensates by re-routing `docker-build` to `ubuntu-latest` so its four legs run fully parallel off the 2-slot fleet (the long pole becomes the ~13.2 min `github-actions` leg rather than the measured 20.3). **This is a measurement claim that only a real post-merge release can settle** — one more reason AC1 is load-bearing and not a formality.

### Finding 3 — the `gobuild` stage has never been executed by any CI run, on any ref (new)

The strongest gap in the chain's evidence. AC3/AC4 rest entirely on static reads:

- `gh run list --workflow push.yaml --branch ci/concurrency-release-fleet-slots` → **`[]`** (zero runs — `push.yaml` doesn't trigger on branches).
- `branch.yaml` contains **no reference** to `github-actions.Dockerfile` or `cloud-helpers.aws.Dockerfile` — branch CI never builds these two images.
- In the branch run `39318720877`, the only docker-ish job is `Build github-actions-staging` → **`skipped`** (and `github-actions-staging.Dockerfile` has no `gobuild` stage anyway).

So no BuildKit has ever parsed these `gobuild` stages, honoured this `.dockerignore`, or resolved the Go module graph inside the pruned context. The developer's probe tested Go ldflags **on the host with the full tree present** — a useful check of the stamp mechanism, but it does **not** exercise the Docker stage, the cache mounts, or `.dockerignore` enforcement. AC3/AC4/AC5 are "correct by inspection", not verified. That is exactly what AC1 was supposed to supply.

## 5. Roadmap — not updated, and it cannot be from here

Confirmed independently: `git ls-tree -r --name-only origin/main | grep -i roadmap` returns only `docs/ROADMAP.md` and four archived `IMPLEMENTATION_ROADMAP.md` files. **There is no `docs/roadmap/items/` in `api` at all.** The target lives in `simple-container-com/forge`. This run's branch is `api`-scoped, so writing it here would create the file in the wrong repo.

Recommended content for whoever holds `forge` write access (the Roadmap Driver owns `docs/roadmap/`):

- `status: in-progress` → **`code-complete`** (**not** `shipped`)
- `blocked_on:` → `"operator: PR api#461 (ci/concurrency-release-fleet-slots, head 0a135681) is OPEN and CI-red — 'Build Setup' step 'fmt' fails on golangci-lint package loading 3m6.27s vs a 3m budget (welder.yaml:97; pre-existing, byte-identical on main, not caused by cf7e9104). Needs the timeout raised or a warm-cache re-run, then human review per branch protection."`
- `next:` → re-verify against the merge commit on `main`, then the first `push.yaml` run on `main`: confirm no `download …binary` steps, confirm the `gobuild` stage actually executes, and confirm the baked version ≠ `latest`/`0.0.0`.
- `## History` += 2026-10-20: AC2/AC5 proven by diff; AC3/AC4 correct by inspection but **never executed** in any CI run; AC1 blocked — no post-#461 release exists. Four SDLC characters reached the same wall.

## 6. Tests run

Zero source files changed this turn, so there was nothing of mine to build or test. Per `CLAUDE.md` I deliberately avoided linking the ~2090-package closures (needs ~13 GB on a 12 GB box).

```
$ git merge-base --is-ancestor cf7e9104 origin/main; echo $?
1                                  # NOT in main

$ git show origin/main:.github/workflows/push.yaml | grep -c "download-artifact@"
5
$ git show 0a135681:.github/workflows/push.yaml | grep -c "download-artifact@"
3                                  # the two binary fetches are gone

# AC4 — real symbol, empirical stamp
$ go run -ldflags "-X=github.com/simple-container-com/api/internal/build.Version=2026.10.20-probe" ./zz_vprobe_main.go
2026.10.20-probe
$ go run ./zz_vprobe_main.go
latest                             # default is "latest", not the brief's "0.0.0"
```

(The probe file `zz_vprobe_main.go` was removed; `git status --porcelain` is empty.)

**Static-lint gate:** `.golangci.yml` is present and `Makefile:102` has a `lint:` target, so the gate applies to this repo. I did **not** run it — **no-op by construction**: zero source files changed, and `git diff origin/main...HEAD` excluding `.workflow/` is empty, so there is nothing for the linter to judge. Noting it explicitly rather than silently skipping. (Separately, Finding 1 shows this repo's lint currently times out in CI regardless of content.)

## 7. Operator action required (precise, in order)

1. **Unblock #461's CI.** Raise `--timeout 3m` → `10m` at `welder.yaml:97`, or re-run on a warm cache. Package loading needs >3m against the ~2143-package closure on a 2-vCPU fleet guest. This is a pre-existing fleet/cold-cache fragility, **not** a defect in the binary-roundtrip fix.
2. **Then merge #461** (`ci/concurrency-release-fleet-slots`, head `0a135681`). SDLC automation cannot — branch protection requires human review (tracked as `api-repo-branch-protection-blocks-sdlc-automerge`).
3. **Then re-dispatch this verification slice.** It is cheap and fully scripted at that point: observe the first `push.yaml` run on `main`, confirm (a) no `download …binary` steps, (b) the `gobuild` stage actually executes in both image legs — **first execution ever**, per Finding 3, (c) the baked version ≠ `latest`/`0.0.0`, and (d) the real wall-clock saving given `docker-build` still `needs: build-binaries` (Finding 2).
4. **Decide the roadmap write path** — the item is in `forge`, unreachable from an `api`-scoped SDLC run. Route it through the Roadmap Driver, or dispatch a `forge`-scoped run.

Note for the operator: one secondary concern worth a glance — `.dockerignore` excludes `docs/`, and `docs/embedfs.go` is a real Go package (`package docs`) imported across `pkg/assistant/*`. The two in-image binaries don't reach it today, so the current state is safe, but the margin is undocumented: any future import pulling `api/docs` into either closure breaks the image build with an opaque "finding module for package" error. A one-line comment in `.dockerignore` would cost nothing — author's call on #461.

## 8. Why `failure`, not `signoff`

Not a judgement on the fix — **the fix looks sound** and I'd expect it to pass once it runs. This is a judgement on *this slice*:

- The brief's central deliverable (AC1 / DoD-a, live post-merge evidence) is **undeliverable** — unrecoverable without operator action, and that action is itself blocked by red CI.
- **Neither** Definition-of-Done item landed: no live evidence, no roadmap update (wrong repo).
- The verdict contract defines `signoff` as "work is complete and meets the brief's ACs." It does not. `failure` = "unrecoverable blocker; the slice cannot finish without a fundamentally different approach" — which here is literally true: it needs a merge this chain cannot perform.
- With `auto-merge`, `signoff` would merge a PR whose **entire diff is `.workflow/` engine metadata** and report "done" upstream — indistinguishable from a fabricated signoff, and the no-op-merge failure mode my gate exists to stop. Escalation to the operator is the correct outcome.

**Empty-PR-OK:** no — I am deliberately **not** declaring it. The exemption requires live evidence for what the brief asked; the brief asked for post-merge release evidence that does not exist, and the AC3/AC4 reads were never executed anywhere (Finding 3). The branch diff is pure engine metadata and **should** escalate rather than auto-merge.

**Verdict:** failure