[sc-stack-outputs-leaks-secrets-in-clear] sc stack outputs prints secret-marked outputs in the clear, with no flag to mask them

## Background

Live-caught during the 2026-10-18 k3s smoke: `sc stack outputs -s <stack>` prints every output value in the clear, including outputs that are secret-marked by the resource — a cluster-admin kubeconfig and a k3s join token were both observed unmasked in the smoke's terminal output. There is no flag to mask or redact secret-marked values — `--show-secrets` exists to control *whether* secret outputs are shown at all, but once shown, they're always plaintext.

A kubeconfig with cluster-admin and a cluster join token are both high-value credentials — once printed to a terminal or CI log, they're effectively leaked (logs are often retained, screen-shared, or pasted). `sc secrets reveal`/`sc secrets hide` already model "sensitive value, handle with care" for the secrets registry; `sc stack outputs` has no equivalent masking behavior for resource outputs marked secret.

## NOTE — prior dispatch of this exact item crashed in the developer character

Run `32521357-2668-4c22-ba01-607e7fca2c71` (2026-10-18) died at the developer character to three repeated `exit status 143` crashes (the same watchdog/OOM-type pattern — `go vet` + the heavy Pulumi SDK compile under memory pressure, per this repo's own CLAUDE.md note on build-gate non-completion due to resource limits). No PR, no commit resulted. If you hit the same resource-exhaustion pattern, scope your build/test commands narrowly (e.g. `go test ./pkg/<touched>/...` rather than `go build ./... && go test ./...`), and report any genuine non-completion honestly rather than silently passing. Do not assume the architecture/design work from that dead run is reusable — start fresh from this brief.

## Task

Add a mask-by-default (or opt-in reveal flag) to `sc stack outputs` for secret-marked resource outputs, mirroring the existing `sc secrets reveal`/`sc secrets hide` pattern:

1. Mask secret-marked outputs by default (show a fixed-width placeholder like `<secret:redacted>`), with an explicit `--show-secrets` (already exists for enabling display at all) or a new `--reveal-secrets`/`--no-mask` flag to opt into plaintext.
2. At minimum, pipe secret-marked outputs through the same masking the CLI already uses elsewhere (if any exists) so a plain `sc stack outputs` invocation never leaks them, only an explicit opt-in does.

## Definition of done

- `sc stack outputs` does not print a secret-marked resource output in the clear unless the invoker explicitly opts in (new or existing flag).
- A smoke re-run against a stack with a secret-marked output (e.g. a k3s cluster resource) confirms the default invocation shows a masked/redacted value, and the opt-in flag reveals it.
- Update `docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md`'s `## Status` + `## History` with the outcome and evidence. If genuinely shipped+live-verified, flip `status: shipped`; if code lands but isn't smoked live, use `status: code-complete` — don't overclaim.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All clean. Here's the handoff.

---

# Handoff — Roadmap Driver (run c7971524)

## 1. Commit + branch identity

- Commit `«sha-from-triggering-run-redacted»` on branch `«branch-from-triggering-run-redacted»`
- Also merged two upstream PRs directly via GitHub API (not commits on this branch): `simple-container-com/forge` PR #1615 (merge commit `«sha-from-triggering-run-redacted»`), and before that, pulled `origin/main` into this branch (merge commit `«sha-from-triggering-run-redacted»`) to pick up #1615's content before editing the same file.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +3/-3 | 3 index rows updated: `api-repo-branch-protection-blocks-sdlc-automerge` → ✅ SHIPPED; `sc-stack-outputs-leaks-secrets-in-clear` → 🚧 IN PROGRESS (re-dispatched); `sc-stack-outputs-empty-byo-vm-host-fields` → 🚧 IN PROGRESS (dispatched) |
| docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md | +13/-10 | front-matter → `status: shipped`; Status table row → ✅ SHIPPED; new `## History` entry recording the reconcile evidence |
| docs/roadmap/items/sc-stack-outputs-leaks-secrets-in-clear.md | +16/-11 | front-matter → `status: in-progress`, `dispatched: "2026-10-18"` (2nd attempt); new `## History` entries for the STEP 1 reconcile (flip to redispatch) and STEP 4 re-dispatch |
| docs/roadmap/items/sc-stack-outputs-empty-byo-vm-host-fields.md | +7/-6 | front-matter → `status: in-progress`, `dispatched: "2026-10-18"`; new `## History` entry for the dispatch |
| docs/roadmap/driver-log/2026-10-18.md | +45/-0 | this run's full run note prepended (newest-first) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile 🚧 IN PROGRESS slices with `dispatched:` set | PASS | Both found: `api-repo-branch-protection-blocks-sdlc-automerge` (run `2e40aa56`) and `sc-stack-outputs-leaks-secrets-in-clear` (run `32521357`). Resolved via `get_run` + GitHub API checks of PR state. |
| AC2 | Flip shipped SDLC PRs | PASS | `api-repo-branch-protection-blocks-sdlc-automerge`: run `2e40aa56` `status: completed`, its `api` PR [#454](https://github.com/simple-container-com/api/pull/454) open, title has no `[escalated]` prefix — matches DevOps's own closing condition exactly. Flipped to `shipped`. |
| AC3 | Detect and requeue stale/failed dispatches | PASS | `sc-stack-outputs-leaks-secrets-in-clear`: run `32521357` `status: failed`, >4h old, developer character crashlooped 3× (`exit status 143`), no PR/commit found on `api` (verified via `gh api` commit/PR search). Flipped to `redispatch`, escalated, re-dispatched. |
| AC4 | Pick next un-done slice(s) | PASS | Both auto-pickable rows in the entire index (every other `⬜`/`🟡` row is `dispatch: manual`) were picked: `sc-stack-outputs-leaks-secrets-in-clear` (redispatch) and `sc-stack-outputs-empty-byo-vm-host-fields` (next). |
| AC5 | Dispatch Main Forge SDLC | PASS | Both `trigger_workflow` calls returned `{queued:true}` against `api` (`6a1df1a2e639099d50d23c32`). |
| AC6 | Mark picked slices IN PROGRESS | PASS | Both items: `status: in-progress`, `dispatched: "2026-10-18"`, `updated: 2026-10-18`, README rows flipped, `## History` lines added. |
| AC7 | Lint clean | PASS | `bash docs/roadmap/lint.sh` → `406 items, 406 index rows, README.md 234 lines — 0 errors, 0 warnings`. |

## 4. Tests run

```
$ bash docs/roadmap/lint.sh
406 items, 406 index rows, README.md 234 lines — 0 errors, 0 warnings
```

## 5. Notes

- **No new intake** this run — the README Inbox was already clear (the prior same-day Driver run filed both stray entries).
- **No archive pass** — the only zero-marker `✅ SHIPPED` candidates (`chat-refusal-masked-as-budget`, `sha-gate-treats-devops-as-developer`, `sc-k3s-cluster-resource`) are all `updated` within the last day, short of the ≥2-day gate.
- **Parallel cap**: N=0 at pick time (both live dispatches were resolved in STEP 1 before STEP 3 ran); picked 2 of the 4-slice cap — no more `auto` rows existed to fill the remaining 2 slots.
- Sent one operator escalation (Telegram, delivered) for the crashlooped `sc-stack-outputs-leaks-secrets-in-clear` dispatch, noting it was already re-queued.

**Verdict:** signoff
workflowTriggerParentRunId: c7971524-3c75-44fd-ab56-e0126c73edcd
workflowTriggerRootRunId: c7971524-3c75-44fd-ab56-e0126c73edcd