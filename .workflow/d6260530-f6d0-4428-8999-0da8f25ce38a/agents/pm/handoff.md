[thinking] # PM Handoff — `api-release-rerun-fails-existing-github-release`

**Slice ID:** `api-release-rerun-fails-existing-github-release` (roadmap item: `docs/roadmap/items/api-release-rerun-fails-existing-github-release.md`, status `in-progress`, dispatched 2026-10-21)

## 0. Restated requirement

`scripts/create-github-release.sh` unconditionally runs `gh release create "${VERSION}" …`. Re-running a release job (the normal recovery path when an image leg hits a transient CDN/registry flake) re-enters this step after the Release object for that CalVer tag already exists, and dies with `a release with the same tag name already exists: <VERSION>`. Every artefact in the pipeline (build, sign, publish, docs deploy) is fine — only this last step is non-idempotent. Fix: make the script idempotent without weakening either of its two existing safety guards (preview/RC short-circuit, unsigned-release refusal).

This is a **small, mechanical, single-file bash fix** — no new dependency, no API change, no data migration. Minimal bet: branch on `gh release view` and swap `create` for `upload --clobber` on the existing-tag path.

## 1. Scope — MUST / NICE-TO-HAVE / CUT

| Item | Classification | Notes |
|---|---|---|
| Detect existing release for `${VERSION}` via `gh release view "${VERSION}" >/dev/null 2>&1` | MUST | This is the entire fix |
| Existing-release branch: `gh release upload "${VERSION}" "${assets[@]}" --clobber` | MUST | Refreshes assets instead of duplicate-create; drop `--generate-notes`/`--verify-tag`/`--title` (upload doesn't take them) |
| Fresh-tag branch: keep existing `gh release create … --title … --generate-notes --verify-tag` unchanged | MUST | No behavioural change to the happy path |
| Preserve unsigned-release refusal (zero `.sigstore.json` → exit 1) ahead of both branches | MUST | Must fire identically on first-run and re-run — do not move it, do not make it conditional on which branch is taken |
| Preserve preview/RC short-circuit (`-pre.` / `-preview.` → exit 0) unchanged | MUST | Already a separate, earlier guard — do not touch |
| Add a test or documented manual verification procedure for both branches | MUST | Per brief's AC5; repo has no existing test harness for this script (confirmed — no bats/shell test framework present, only `scripts/verify-preview.sh` which is a different live-verification script, not a unit test). A **bats test is NICE-TO-HAVE**; the **MUST** is a clear manual verification procedure (this repo's existing pattern for scripts/ is manual/live verification, e.g. `verify-preview.sh`'s own docstring) |
| Introduce a shell test framework (bats) into the repo for this one script | CUT | Disproportionate for a ~5-line fix; no precedent in `scripts/` — follow the repo's existing convention (documented manual procedure) rather than importing new tooling |
| Touch `.intoto.jsonl` dual-publish logic, asset glob list, or notes generation | CUT | Out of scope per brief; not part of the reported failure |
| Idempotency for `welder run tag-release` / `welder deploy -e prod` steps elsewhere in `push.yaml` | CUT | Brief explicitly scopes to this one script; those steps are already reported green on re-run |

## 2. Acceptance criteria (restated from brief, as a checklist for Architect/Developer/QA)

| AC# | Criterion |
|---|---|
| AC1 | A re-run of a release run whose GitHub Release already exists finishes green (exit 0) |
| AC2 | On that re-run, existing assets are refreshed via `gh release upload --clobber` rather than the job dying on duplicate-tag `gh release create` |
| AC3 | The unsigned-release refusal (zero `.sigstore.json` sidecars → `exit 1` with the `::error title=No SLSA provenance::` message) is unchanged and still fires on a re-run, before either create or upload is attempted |
| AC4 | Preview/RC tag short-circuit (`*-pre.*` / `*-preview.*` → `exit 0` with the "Skipping Release creation" message) is unchanged |
| AC5 | A test (preferred: bats, if introducing one is cheap) or a clear, explicit manual verification procedure exists covering both branches: (a) fresh tag → `gh release create`, (b) pre-existing tag → `gh release view` succeeds → `gh release upload --clobber` |

## 3. Design guidance for Architect (non-binding, PM's suggested shape)

Current script flow (`scripts/create-github-release.sh`):
1. L1-41: header/env checks — unchanged.
2. L~43-48 (`-pre.`/`-preview.` check) — unchanged, AC4.
3. L~58-67 (`.sigstore.json` count guard) — unchanged, keep this **before** the create/upload branch so it gates both paths identically, AC3.
4. L~69-86 (`.intoto.jsonl` dual-publish + `assets=()` collection) — unchanged; both `create` and `upload` need the same `assets[]`.
5. L~110 (`gh release create …`) — **replace this single call** with:
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
   Note: `gh release upload` does not accept `--title`/`--generate-notes`/`--verify-tag` — the existing Release object already carries those from the first successful run, so there's nothing to regenerate. `--clobber` overwrites same-named assets; it does not duplicate them.
6. Exit code: `gh release upload` returns non-zero on real failure, so `set -euo pipefail` still protects the re-run against a genuinely broken upload — only the "already exists" case is absorbed.

## 4. Verification plan (hand to Developer/QA)

Manual procedure (append to the script's header comment or to `docs/RELEASES.md`):
1. **Fresh-tag path**: pick a tag that has never had a Release (or delete a test Release first: `gh release delete <tag> --yes`), run the script with a dist bundle containing ≥1 `.sigstore.json` → expect `gh release create` path, exit 0, Release visible at `/releases/tag/<tag>`.
2. **Existing-tag path**: run the script again for the *same* tag/dist bundle → expect the `gh release view` branch to fire, `gh release upload --clobber` to run, exit 0, no "already exists" error, assets re-uploaded (check `updated_at` on the release moves).
3. **Unsigned regression check**: run with a dist bundle that has zero `.sigstore.json` sidecars, for both a fresh and an existing tag → expect exit 1 with the `No SLSA provenance` error in **both** cases (this is the critical regression guard per the brief — verify it wasn't accidentally scoped only to the create branch).
4. **Preview tag regression check**: run with `VERSION` containing `-pre.` or `-preview.` → expect exit 0, "Skipping Release creation" message, no `gh` call at all.
5. If a bats test is added instead/in addition, it should mock `gh` (stub script on `PATH` returning controlled exit codes for `release view`) to exercise all four cases above without hitting real GitHub.

## 5. Out-of-scope / explicitly not this slice

- Any change to `.github/workflows/push.yaml` itself (the `env:`/`run:` invocation shown in `docs/RELEASES.md` stays the same — the script is called identically either way).
- Any change to asset glob patterns, SBOM/provenance generation, or the `.intoto.jsonl` dual-publish shim.
- Hardening around `welder run tag-release` / `welder deploy -e prod` idempotency — those already succeed on re-run per the bug report.

## 6. Files touched

No code mutations in this turn — PM plan only.

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 7. Acceptance criteria verification

Not applicable to this turn (no implementation yet) — table above (§2) is the checklist Architect/Developer/QA must satisfy; PM cannot PASS/FAIL these before code exists.

## 8. Tests run

None — no code mutated in this turn.

## 9. Handoff to Architect

- Single file: `scripts/create-github-release.sh`.
- The fix is ~10 lines of bash (see §3 sketch) — Architect should confirm the `gh release view`/`gh release upload --clobber` approach against current `gh` CLI semantics (both subcommands have been stable for years; no version pinning concern expected but worth a one-line check of the `gh` version pinned in `push.yaml`'s runner image) and then hand straight to Developer.
- Developer: implement §3, update the script's header comment (exit-codes section currently says "0 Release created (or skipped…)" — extend to mention "or assets refreshed on existing tag"), add the manual verification steps from §4 either inline as a comment block or in `docs/RELEASES.md`.
- QA: run through §4's four scenarios (or review the bats test if Developer adds one) before signoff; specifically do not let AC3 (unsigned refusal) regress — it must fire on *both* branches.
- Roadmap: when this ships, the roadmap item `api-release-rerun-fails-existing-github-release` should move `in-progress` → `shipped` per the repo's roadmap conventions (not PM's job this turn, but flagging for whichever downstream role closes the loop — likely DevOps/Architect per the Roadmap Driver's own convention of letting the SDLC run report back).

**Verdict:** signoff