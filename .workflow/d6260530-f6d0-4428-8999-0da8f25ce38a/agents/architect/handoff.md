[thinking] # Architect Handoff — `api-release-rerun-fails-existing-github-release`

**Slice ID:** `api-release-rerun-fails-existing-github-release`
**Role transition:** pm → architect → (next: developer)

## 0. Context verified this turn

I read the live file directly from the repo (not from the brief or PM's description) to confirm PM's restated script shape is accurate:

- `read_repository_file(simple-container-com/api, scripts/create-github-release.sh)` — content matches PM's line-by-line description exactly (env checks L1-41, preview/RC short-circuit at `if [[ "${VERSION}" == *"-pre."* || "${VERSION}" == *"-preview."* ]]`, `.sigstore.json` zero-count refusal via `arr_sigstores=( sc-*-v${VERSION}.tar.gz.sigstore.json )` → `exit 1` with `::error title=No SLSA provenance::...`, `.intoto.jsonl` dual-publish loop, `assets=()` collection, final unconditional `gh release create "${VERSION}" "${assets[@]}" --repo ... --title "v${VERSION}" --generate-notes --verify-tag`).
- `docs/RELEASES.md` confirms the same `gh release create` invocation is also quoted there as documentation of the pipeline (so that doc needs a one-line update too, not just the script — see §3).
- `.github/workflows/push.yaml` (fetched via raw.githubusercontent.com, truncated at the matrix-build step but far enough to confirm) shows `concurrency: cancel-in-progress: false` for this workflow — confirms PM's framing that a re-run is a deliberate, supported recovery path, not an accidental double-trigger; this is a real design input (idempotency is a first-class requirement here, not a nice-to-have).
- `gh release upload` and `gh release view` are documented, stable GitHub CLI subcommands (manual pages exist at `cli.github.com/manual/gh_release_upload` and `.../gh_release_view`) — confirms PM's approach uses real CLI surface, not a guessed flag. I did not find a pinned `gh` version in `push.yaml`'s visible portion; the runner image (`blacksmith-8vcpu-ubuntu-2204` / GitHub-hosted fallback) ships a current `gh` via its default toolcache, and `--clobber` on `gh release upload` has been stable since `gh` v1.x — no version-pinning risk flagged.

No code mutations in this turn — this is architecture review/design, same as PM's two prior turns. **`git log -1` verification is N/A**: I touched no files on the real path `scripts/create-github-release.sh`.

## 1. Commit + branch identity

No code mutations in this turn — architecture review only. Nothing to cite.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

(Empty — Architect role, no implementation this turn.)

## 3. Design decision — confirming and tightening PM's shape

PM's sketch (§3 of their handoff) is correct and I'm **approving it as-is** with two small refinements for Developer:

**Refinement A — repo flag consistency.** PM's sketch passes `--repo "${GITHUB_REPOSITORY}"` to both `gh release view` and the two `gh release`/`upload` calls. Confirm this stays — the live script already passes `--repo "${GITHUB_REPOSITORY}"` to `create`; Developer must add the same flag to the new `view` and `upload` calls so the idempotency check is scoped to the correct repo under `workflow_dispatch` manual backfill use (per the script's own header comment: "safe to invoke from anywhere").

**Refinement B — don't swallow a real `gh release view` failure silently into the create branch.** `gh release view` exits non-zero both when the release doesn't exist (404 — the case we want) AND on a transient API error (rate-limit, auth hiccup). Because the script is piping `view`'s stderr to `/dev/null` via `>/dev/null 2>&1`, a transient `view` failure will silently fall through to the `create` branch — which is actually fine here (worst case: `create` then fails loudly with `set -euo pipefail` because the tag/release truly does exist, or succeeds because it truly doesn't). This is **acceptable, not a design gap** — don't over-engineer a retry/backoff around `gh release view` for this fix; the downstream `create`/`upload` call is still the authoritative action and still fails loudly on a genuine problem. Flagging this explicitly so Developer doesn't feel compelled to add retry logic PM/brief didn't ask for.

Final shape for the replacement of the current L~110 `gh release create` call (unchanged from PM, confirmed against live file):

```bash
if gh release view "${VERSION}" --repo "${GITHUB_REPOSITORY}" >/dev/null 2>&1; then
  echo "Release ${VERSION} already exists — refreshing assets (re-run recovery path)."
  gh release upload "${VERSION}" "${assets[@]}" \
    --repo "${GITHUB_REPOSITORY}" \
    --clobber
else
  gh release create "${VERSION}" "${assets[@]}" \
    --repo "${GITHUB_REPOSITORY}" \
    --title "v${VERSION}" \
    --generate-notes \
    --verify-tag
fi
```

Everything above this block (env checks, preview/RC short-circuit, `.sigstore.json` refusal, `.intoto.jsonl` dual-publish, `assets=()` collection) is **untouched** — confirmed from the live file read that these are all strictly earlier in the script than the single `gh release create` call being replaced, so the guard ordering PM specified is already correct in the current file layout; Developer does not need to reorder anything, only replace the final call.

### Also update (documentation consistency, small, same PR)
- `docs/RELEASES.md` currently quotes the bare `gh release create ...` block verbatim as "what the script does." Developer should update that quoted block to show the branch (or replace it with prose: "creates the Release on a fresh tag, or refreshes assets via `gh release upload --clobber` on a re-run") so the doc doesn't drift from the script it documents. This is NOT a new scope item — it's keeping an existing doc's code-quote in sync with the file it quotes, same spirit as CLAUDE.md's own "avoid doc/code drift" ethos.
- Script header comment's **Exit codes** section (`0 Release created (or skipped because tag is preview/RC)`) should gain "...or assets refreshed on an existing release" per PM's §9 note — one-line comment edit, Developer's call on exact wording.

## 4. Acceptance criteria — design-level review (not yet implemented; PASS/FAIL deferred to QA post-implementation)

| AC# | Criterion | Design status | Rationale |
|---|---|---|---|
| AC1 | Re-run of an already-released tag finishes green | DESIGN-SOUND | `gh release view` success → `upload` path; `upload` returns 0 on success; `set -euo pipefail` still active so a genuine upload failure still fails the job (correct — AC1 wants "finishes green when it legitimately can", not "always exit 0") |
| AC2 | Existing assets refreshed via `upload --clobber`, not duplicate-create | DESIGN-SOUND | `--clobber` is gh's documented flag for overwriting same-named assets on an existing release (confirmed via `cli.github.com/manual/gh_release_upload`); no duplicate-create path is reachable once `view` succeeds |
| AC3 | Unsigned-release refusal unchanged, fires on re-run before create/upload | DESIGN-SOUND | Confirmed from the live file: the `.sigstore.json` zero-count guard sits earlier in the script (before `assets=()` collection, long before the final call), so it is **structurally unconditional** — it runs identically whether the eventual branch is create or upload. The design does not touch or move this guard. |
| AC4 | Preview/RC short-circuit unchanged | DESIGN-SOUND | Confirmed from the live file: the `-pre.`/`-preview.` check is the very first logic after the env-var assertions, `exit 0`s immediately, and never reaches the sigstore guard or the create/upload branch. The design does not touch this block at all. |
| AC5 | Test or manual verification procedure for both branches | DEFERRED TO DEVELOPER/QA | PM's §4 four-scenario manual procedure (fresh-tag create, existing-tag upload, unsigned-refusal-on-both-branches, preview-short-circuit) is the right shape and matches this repo's existing convention of documented manual verification over new test tooling (confirmed: `scripts/` has only `create-github-release.sh` + `verify-preview.sh`, no bats/shell-test harness present in the repo). Developer should write this as a comment block in the script or append to `docs/RELEASES.md`; a bats test remains NICE-TO-HAVE per PM, not MUST. |

## 5. Tests run

None — no code mutated in this turn (architecture review only). `read_repository_file` confirms the current script content cited above is live/accurate as of this turn, not assumed.

## 6. Handoff to Developer

- Single-file change: `scripts/create-github-release.sh`, replacing only the final `gh release create` invocation with the if/else block in §3 above. No other lines move.
- Companion doc touch: `docs/RELEASES.md`'s quoted `gh release create` block (small, same PR).
- Add the manual verification procedure (PM §4, four scenarios) as a header comment block in the script and/or an addition to `docs/RELEASES.md`'s "How release notes are produced" section.
- Run `bin/gofumpt` / lint is N/A for a `.sh` file; no Go build/vet gate applies to this change (per CLAUDE.md, those gates are for the Go module, not `scripts/`). Developer should instead do a **local dry-run** of the bash logic (e.g. `bash -n scripts/create-github-release.sh` for a syntax check, plus manually invoking with a stub `gh` on `PATH` per PM's §4.5 suggestion) since there's no CI for this script specifically.
- QA: verify AC3 and AC4 do not regress — these are the two guards this slice must NOT touch, and my read of the live file confirms they sit structurally before/independent of the new branch, so a correct implementation cannot accidentally gate them on which branch fires. If QA's test run shows otherwise, that's an implementation bug, not a design gap.
- Roadmap: on ship, move `docs/roadmap/items/api-release-rerun-fails-existing-github-release.md` `in-progress` → `shipped` (not this turn's job — flagging for whichever role closes the loop, per Roadmap Driver convention).

**Verdict:** signoff