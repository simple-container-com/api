[sc-yc-deploy-fidelity] A green YC deploy must mean a running container
Target repository: api (Simple Container core SC provisioner; changes can affect all YC-deployed consumers). S1+S2 only; leave the general provider-warning rendering question (S3) for research. Diagnose and fix the green-deploy/zero-revision failure in pkg/clouds/pulumi/yandex/serverless_container.go: terraform-provider-yandex turns a failed DeployRevision into diag.Warning, so SC exits 0 even when the container has no revision and serves 404. Assert after both create and update that the container has a deployed revision, fail the deploy with actionable container/folder identity and underlying YC diagnostic when possible; ensure first revision waits for required service-account IAM bindings (container-registry.images.puller). Filter YC-reserved environment names, notably PORT, out of the environment sent to YC while preserving AWS Lambda's shared env anchor; document/log dropped names without leaking values. Acceptance criteria: (1) deliberately broken revision fails SC deploy rather than green/404 and surfaces the YC failure; (2) shared client.yaml containing PORT deploys unchanged on YC, with PORT omitted from revision env; (3) regression tests cover failed create/update and reserved env filtering, including IAM dependency; (4) build/test api and at least one dependent service (forge-atrium is the YC adopter), then deploy and live-smoke via GitHub Actions when authorized, otherwise distinguish code-complete from shipped and name exact operator gate. Do not run sc deploy locally or expose secrets. Design context: forge/docs/design/2026-09-29/atrium-on-yandex-cloud/README.md §12.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
## Roadmap Driver handoff

1. **Reconciliation — PASS.** Four slices already occupy the live-dispatch cap: `briefing-templates-as-data` (PR #524 open), `skills-delivery-opaque` (PR #540 open), and two runs dispatched today and active in QA. No merged titled PR was found for them. There were no stray Inbox entries or archive-eligible items.

2. **Dispatch — FAIL.** I mistakenly queued two follow-up workflows despite the cap of four. `trigger_workflow` reported `queued:true`, but these follow-ups fire **only if this Driver run succeeds**. I reverted both roadmap rows to `⬜ NEXT` with empty `dispatched:` values and am returning failure so neither follow-up fires. The next Driver run can pick them when capacity opens.

3. **Files touched.** Branch: `«branch-from-triggering-run-redacted»`. No commit was made in this turn.

   | Path | Lines added / removed | What changed |
   |---|---:|---|
   | `docs/roadmap/driver-log/2026-10-08.md` | +8/-0 | Recorded reconciliation, cap error, and reverted marks |
   | `docs/roadmap/items/sc-yc-deploy-fidelity.md` | +2/-1 | Recorded the erroneous queue; retained `next` |
   | `docs/roadmap/items/call-to-report-portal.md` | +2/-1 | Recorded the erroneous queue; retained `next` |

4. **Acceptance criteria verification.**

   | AC# | Criterion | Status | Evidence |
   |---|---|---|---|
   | AC1 | Reconcile in-progress slices | PASS | GitHub PR checks and live run-state checks recorded in the driver log |
   | AC2 | Pick and dispatch within the parallel cap | FAIL | Four live dispatches already filled the cap; two follow-ups were queued in error |
   | AC3 | Mark valid dispatches in progress | FAIL | Marks were reverted because the queued follow-ups must not fire |

5. **Verification.** `bash docs/roadmap/lint.sh` → `359 items, 359 index rows, README.md 205 lines — 0 errors, 0 warnings`. `git diff --check` passed.

**Verdict:** failure
workflowTriggerParentRunId: aa10305d-d3d9-4f1e-903c-7a7fa62d813e
workflowTriggerRootRunId: aa10305d-d3d9-4f1e-903c-7a7fa62d813e