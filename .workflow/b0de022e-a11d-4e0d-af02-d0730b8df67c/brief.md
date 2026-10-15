[aws-provider-key-in-deploy-log] SC prints secret-marked provider credentials into the Actions log

## Goal
A failed isolated deploy (`forge` run 37628464539, job Deploy forge-baas (isolated)) printed a live AWS access key and its secret in plaintext, 4x in one log, as part of a `getResource Invoke RPC finished` diagnostic.

Marking the value secret does not prevent this: the payload shows Pulumi's secret envelope around the value (the `4dabf181…` / `1b470612…` special signatures), i.e. the property WAS marked and Pulumi's own summaries would have redacted it. The leak is in SC itself: `api/pkg/clouds/pulumi/events.go:75` (`procDiagnosticEvent`) forwards `evt.DiagnosticEvent.Message` verbatim at Info level, and that message is the raw gRPC payload. So the exposure is generic — any secret property of any resource or provider, on any cloud, can print this way.

## Scope — S3 ONLY
S1 (stop using a human's key for deploys) and S2 (revoke the leaked key) are already done / an operator-timed judgement call respectively — do NOT touch IAM, do NOT revoke any credential. This dispatch is S3 only: redact in `procDiagnosticEvent`, the one chokepoint every provider's diagnostics pass through.

`applyAWSProviderCreds` at `pkg/clouds/pulumi/aws/provider.go:96-99` also passes `args.AccessKey`/`args.SecretKey` as plain `sdk.String` (unmarked) — note it, but the real fix belongs in `procDiagnosticEvent` since that's generic across every provider/cloud, not just AWS.

## Definition of done
- `procDiagnosticEvent` redacts secret-shaped values (Pulumi's secret envelope signatures `4dabf181…`/`1b470612…`, and/or common credential patterns) before logging `evt.DiagnosticEvent.Message` at Info level.
- A deploy log from a current SC binary contains no credential material for any provider.
- The redaction is a property of `procDiagnosticEvent` itself, not of each provider remembering to mark its inputs.
- Unit test(s) prove a secret-marked diagnostic message is redacted before it would reach the log.

## Important — this repo has special merge handling
`simple-container-com/api` carries an active branch-protection ruleset requiring 2 human reviews before merge, which blocks SDLC auto-merge. A fix for this (forge-conductor commit `00c06a29`, deployed 2026-10-15) teaches the dispatch/merge-evidence logic to detect this and fall back to a normal, non-escalated open PR instead of attempting auto-merge and alarming. This dispatch is also the first live test of that fix — if it completes with a clean, safe diff, expect the run to end with an open PR (not an auto-merge attempt) and no escalation on "auto-merge did not land".

## Acceptance criteria
- AC1: `procDiagnosticEvent` redacts secret-shaped values before logging.
- AC2: Unit test covers the redaction against a mocked diagnostic message containing a Pulumi secret-envelope-wrapped value.
- AC3: `go build ./... && go test ./...` (scoped to the touched package) green.
- AC4: Live or named-operator-owed confirmation that a real deploy log no longer contains credential material.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
# Roadmap Driver Handoff — 2026-10-15 run (a6b020a6)

## 1. Commit + branch identity

No commit yet — the harness commits automatically on this run's branch:
`«branch-from-triggering-run-redacted»`.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/roadmap/README.md` | +8/-8 | Flipped `startrun-linked-repo-org-check` → ✅ SHIPPED; `api-repo-branch-protection-blocks-sdlc-automerge` → 🟡 CODE-COMPLETE; `llms-txt-wrong-links-sitemap-gaps` and `aws-provider-key-in-deploy-log` → 🚧 IN PROGRESS |
| `docs/roadmap/items/startrun-linked-repo-org-check.md` | +20/-6 | Reconciled: PR #584 merged + deployed + live-smoked by QA → `status: in-progress` → `shipped` |
| `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +21/-5 | Reconciled: PR #583 merged + deployed, but DoD's live-proof clause is operator/next-dispatch-owed per QA → `status: in-progress` → `code-complete` (not `shipped`) |
| `docs/roadmap/items/llms-txt-wrong-links-sitemap-gaps.md` | +12/-6 | Picked + dispatched; `status: next` → `in-progress`, `dispatched: "2026-10-15"` |
| `docs/roadmap/items/aws-provider-key-in-deploy-log.md` | +16/-8 | Picked + dispatched; `status: next` → `in-progress`, `dispatched: "2026-10-15"` |
| `docs/roadmap/items/sc-vm-compose-single-vm.md` (new) | +34/-0 | Recreated — README row linked it, file never existed (pre-existing lint break, commit `«sha-from-triggering-run-redacted»`) |
| `docs/roadmap/items/sc-k3s-cluster-resource.md` (new) | +33/-0 | Same — recreated from row content |
| `docs/roadmap/items/sc-yc-managed-k8s.md` (new) | +34/-0 | Same — recreated from row content |
| `docs/roadmap/items/sc-ru-clouds-beyond-yandex.md` (new) | +34/-0 | Same — recreated from row content |
| `docs/roadmap/items/sc-ru-market-readiness.md` (new) | +35/-0 | Same — recreated from row content |
| `docs/roadmap/driver-log/2026-10-15.md` | +83/-1 | Appended this run's log (newest-first, above the prior run's entry) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile 🚧 IN PROGRESS slices against GitHub | PASS | Checked both dispatched rows via GitHub API: PR #584 (`startrun-linked-repo-org-check`) merged `02:09:37Z`, real 27-file/+1390 diff, QA live-smoked on an isolated stack, Deploy run `38669517148` success matching merge SHA → `shipped`. PR #583 (`api-repo-branch-protection...`) merged `01:28:50Z`, real 33-file/+1602 diff, Deploy run `38674922361` success matching merge SHA, but QA's own table flags the live-proof DoD clause as operator-owed → `code-complete`, not `shipped` |
| AC2 | Pick the next un-done/un-in-progress slice | PASS | Scanned all 3 horizon tables programmatically for auto-pickable `⬜/🟡` rows; found 2 left queued from the prior run (`llms-txt-wrong-links-sitemap-gaps`, `aws-provider-key-in-deploy-log`) after it hit the platform fan-out cap |
| AC3 | Dispatch Main Forge SDLC | PASS | Both dispatched: `landing` (`6a21c9a09c46098687bc360e`) → `{queued:true}`; `api` (`6a1df1a2e639099d50d23c32`) → `{queued:true}`. No fan-out cap hit this run |
| AC4 | Mark picked slices IN PROGRESS | PASS | Both flipped to 🚧 IN PROGRESS with `dispatched: "2026-10-15"` in item file + README row |

## 4. Tests run

`bash docs/roadmap/lint.sh` → `392 items, 392 index rows, README.md 229 lines — 0 errors, 0 warnings`.

Before my edits, lint failed with 5 pre-existing errors (`row links items/sc-*.md, which does not exist`) traced via `git log -S`/`git show` to commit `«sha-from-triggering-run-redacted»` (already on `main`, unrelated to this run's dispatches) — 5 Mid-term README rows referenced item files that were never created. Fixed by recreating the 5 files from their row content, `dispatch: manual` on each (none are SDLC-dispatchable as filed — new SC provider/resource decisions or research scoping).

## 5. Notable finding

Richer source material for the 5 recreated `sc-*` items likely exists in the `simple-container-com/research` repo (open PRs on RU market entry — dual-market-payments, RU naming registry, Yandex Cloud deployment research) rather than my reconstructed stubs. Worth a follow-up pass to merge that content in rather than treating the stubs as final.

**Empty-PR-OK:** no — this run mutated real roadmap files (2 reconciled items shipped/code-complete, 2 dispatched items, 5 recreated items fixing a pre-existing lint break, driver log) and triggered two live SDLC runs.

**Verdict:** signoff
workflowTriggerParentRunId: a6b020a6-2bca-493e-8cd1-ea68bf7135a5
workflowTriggerRootRunId: a6b020a6-2bca-493e-8cd1-ea68bf7135a5