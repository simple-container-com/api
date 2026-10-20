[api-release-binary-roundtrip] Kill the 15-minute binary artifact round-trip in api's release pipeline

## Background

The operator, watching release run 39253348066, flagged that `Download github-actions binary` and `Download cloud-helpers binary` steps in `docker-build` were taking ages — ~448s each to re-fetch a ~70MB stripped Go binary that `build-binaries` had just compiled minutes earlier. Roughly 15 minutes per release spent fetching bytes across a fleet→GitHub-artifact-store→hosted-runner round trip.

## What already landed (api#461, commit cf7e9104, 2026-10-19)

A digest-pinned `gobuild` stage was added to both Dockerfiles (`github-actions` and `cloud-helpers-aws` image legs) that builds the binary in-image with the exact same flags and `VERSION` build-arg the old `build-binaries` job used. The `dist/*` artifact upload/download round-trip is gone from `push.yaml`; `build-binaries` now stays as a compile-only gate. The same PR also added the repo's first `.dockerignore` as an ALLOWLIST (the previous ungated build context was shipping `.sc/` — decrypted secrets after `sc secrets reveal` — plus `docs/` and `.git/` to the daemon on every image build).

## Your job: verify, not build

This is a **verification-only slice**. The code fix already merged. What's owed is proof that it actually works on the first real post-merge release:

1. Find the first `api` release/deploy run that ran AFTER api#461 (`cf7e9104`) merged to `main`.
2. Confirm in that run's logs:
   - The `download cloud-helpers binary` / `download github-actions binary` steps (and the `dist/*` artifact upload from `build-binaries`) are GONE from the workflow — not just fast, but structurally absent.
   - Both `github-actions` and `cloud-helpers-aws` images build their binary via the new in-image `gobuild` stage.
   - The built binary inside each final image reports the correct version via `-X main.version=${VERSION}` (or whatever the actual ldflags variable is — check the Dockerfile) — i.e. `VERSION` built-arg flows through correctly, not the Dockerfile's `0.0.0` default.
3. Confirm the new `.dockerignore` is doing its job: the build context for these images does NOT include `.sc/`, `docs/`, or `.git/`.
4. If anything regressed (binary round trip crept back, version stamping broke, or `.dockerignore` isn't actually excluding those paths), fix it and note what you found.

## Definition of done

- Live evidence (workflow run URL/log excerpt) that a real post-#461 release has no `dist/*` binary-artifact download step and the images build correctly with the right version baked in.
- Roadmap item `docs/roadmap/items/api-release-binary-roundtrip.md` updated: if verification passes, flip `status: code-complete` → `status: shipped`, add a `## Status` row with the evidence (run URL + what was checked), update `next:` and the README index row to `✅ SHIPPED`.
- If verification is blocked by something operator-only (e.g. you cannot trigger/observe a real release run), stop at code-complete, say exactly what's needed, and do NOT mark shipped.

Full context and the original measurement (448s per binary, two images, ~15 min/release) is in `docs/roadmap/items/api-release-binary-roundtrip.md` in the `forge` repo if you need the detail — but the fix is already merged; your job is the live-verification half only.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
# Roadmap Driver — handoff

## No code mutations in this turn (roadmap-only edits)

This is the Roadmap Driver's own run — it edits `docs/roadmap/` metadata and dispatches a separate SDLC workflow run; it does not touch service source code itself. No commit SHA to report yet (the engine commits/pushes this run's branch automatically); files touched are listed below.

### 1. Commit + branch identity
No code mutations — this run only edited roadmap index/item/log files + dispatched one SDLC run via `trigger_workflow`. Branch: `«branch-from-triggering-run-redacted»` (as given).

### 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +0/-2 (net −4 incl. formatting) | Removed archived rows for `sc-vm-compose-single-vm` + `sc-k3s-cluster-resource`; flipped `api-release-binary-roundtrip` row to `🚧 IN PROGRESS` with dispatch note |
| docs/roadmap/archive/README.md | +2/-0 | Added archive rows for `sc-vm-compose-single-vm` + `sc-k3s-cluster-resource` with evidence + residual |
| docs/roadmap/items/api-release-binary-roundtrip.md | +12/-3 | `status: code-complete`→`in-progress`, `dispatched: "2026-10-20"`, `updated`, new `## History` line |
| docs/roadmap/items/sc-k3s-cluster-resource.md | +1/-1 | `archived: "2026-10-20"` set |
| docs/roadmap/items/sc-vm-compose-single-vm.md | +1/-1 | `archived: "2026-10-20"` set |
| docs/roadmap/driver-log/2026-10-20.md | new file | Run notes: intake, reconcile, archive, pick, dispatch, mark, lint |

### 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | STEP 0 — intake stray Inbox entries | PASS | `## Inbox` section (README.md:237) was empty — nothing to file |
| AC2 | STEP 1 — reconcile `🚧 IN PROGRESS` dispatches against GitHub | PASS | Enumerated all 26 `🚧 IN PROGRESS` rows; every item file has `dispatched: ""` (no live SDLC run to reconcile — confirmed via grep across all 26 item files, not sampled) |
| AC3 | STEP 2 — archive finished items | PASS | `sc-vm-compose-single-vm` + `sc-k3s-cluster-resource`: `status: shipped`, 0 outstanding markers, `updated: 2026-10-18` (≥2 days old), not touched this run → archived (item files kept, `archived:` set, rows moved README→archive/README) |
| AC4 | STEP 3 — pick up to 4 pickable+auto slices | PASS | Exhaustively checked every `⬜ REDISPATCH`/`⬜ NEXT`/`🟡 CODE-COMPLETE` row's item-file `dispatch:` field; exactly one (`api-release-binary-roundtrip`) is `dispatch: auto` — picked it (not a priority choice, the only option) |
| AC5 | STEP 4 — dispatch Main Forge SDLC | PASS | `trigger_workflow` → workflow `99171cbe-7c27-4976-a999-931b834b5d62`, repo `api` (`6a1df1a2e639099d50d23c32`) → `{queued:true}` (tool result above) |
| AC6 | STEP 5 — mark picked slice IN PROGRESS | PASS | `items/api-release-binary-roundtrip.md`: `status: in-progress`, `dispatched: "2026-10-20"`; README row flipped to `🚧 IN PROGRESS` |
| AC7 | STEP 6 — log the run | PASS | `docs/roadmap/driver-log/2026-10-20.md` created with full account |
| AC8 | STEP 7 — lint must pass | PASS | `bash docs/roadmap/lint.sh` → `411 items, 411 index rows, README.md 235 lines — 0 errors, 1 warnings` (pre-existing warning on summary length, unrelated) |

### 4. Tests run
N/A — no service code changed. Verification was `bash docs/roadmap/lint.sh`, run after all edits: `0 errors, 1 warnings`.

### 5. Summary

Reconcile found nothing in flight (all 26 `🚧 IN PROGRESS` items are part-built milestones with no live dispatch). Archived two shipped-and-clean items (`sc-vm-compose-single-vm`, `sc-k3s-cluster-resource`) that cleared the 2-day gate this run. Picked and dispatched the only auto-pickable slice in the entire ~400-item index — `api-release-binary-roundtrip` (verify the already-merged in-image binary-build fix on the first post-merge `api` release) — to Main Forge SDLC against `api`, and marked it `🚧 IN PROGRESS`. Lint is clean.

**Empty-PR-OK:** no — this run's diff is entirely `docs/roadmap/` content (README/archive/items/driver-log), which is this character's real, in-scope output, not engine metadata; nothing to declare under the verification-only exemption.

**Verdict:** signoff
workflowTriggerParentRunId: 06dccbe4-9afb-4aff-8271-7a0715cbcbcc
workflowTriggerRootRunId: 06dccbe4-9afb-4aff-8271-7a0715cbcbcc