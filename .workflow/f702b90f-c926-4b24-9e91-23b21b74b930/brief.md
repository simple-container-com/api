[yc-provider-key-in-deploy-log] The first YC deploy prints the service-account private key into the Actions log
Implement S3 only: audit `api/pkg/clouds/pulumi/*/provider.go` and associated provider constructors/registrars for credentials passed to Pulumi as plain property values instead of secret-marked inputs. Fix every confirmed analogous path with `sdk.ToSecret` at the boundary and regression tests against mock monitor output. Reassess whether forwarding Pulumi preview/update summaries verbatim to stdout risks disclosing any credential values; if needed, make the minimal redaction fix without hiding useful non-secret diagnostics. S0-S2 are already shipped: api PR #421 (`0aaefb04`, SC v2026.9.16) secret-marked the Yandex provider's serviceAccountKeyFile, StorageSecretKey, and YmqSecretKey in both constructors, and the exposed credentials have already been rotated and revoked. Do not repeat that rotation or quote any credential value.
Definition of done: inspected provider paths and findings are documented with file-grounded evidence; any analogous credential fields are secret-marked with tests; an appropriate deploy/smoke verifies credentials remain masked in a fresh preview/update (count secret markers only, never print log lines containing keys). api is the Simple Container core, not just Forge: all six Forge services depend on its deploy machinery, so build/test and smoke at least one dependent service or isolated stack, not only api unit tests. Keep changes minimal and do not run a local sc deploy; use the prescribed CI/deploy path.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
## Roadmap Driver handoff

1. **MUST — Reconcile and dispatch.** No Inbox entries needed filing. `briefing-templates-as-data` remains in progress with forge-conductor PR #524 open. The `skills-delivery-opaque` run failed at QA checkout and had no PR or default-branch commit more than four hours after dispatch; it was re-queued. Two Main Forge SDLC follow-ups returned `queued:true`: the skills retry in **forge-conductor** and the S3 provider-credential audit in **api**. Both item files and index rows are marked `🚧 IN PROGRESS`.
2. **MUST — Next Driver run.** `leave-meeting-fails-missing-scope` was not dispatched: the platform refused a third follow-up with `max follow-up fan-out (2) reached for this run`. It is marked `⬜ REDISPATCH` with no dispatch timestamp; pick it first next run. The stale-skills Telegram escalation was attempted twice, but both calls returned `scheduled:false`; **delivery is unconfirmed**.
3. **CUT — Archive.** No item met the archive gate this run.

**Branch:** `«branch-from-triggering-run-redacted»`. Roadmap documentation was edited; no feature code was changed.

| Path | Lines added / removed | What changed |
|---|---:|---|
| `docs/roadmap/README.md` | +3/-3 | Mirrored three item states |
| `docs/roadmap/driver-log/2026-10-07.md` | +9/-0 | Recorded reconciliation, dispatches and gates |
| `docs/roadmap/items/skills-delivery-opaque.md` | +3/-1 | Recorded failed run and queued retry |
| `docs/roadmap/items/yc-provider-key-in-deploy-log.md` | +5/-4 | Marked S3 dispatched |
| `docs/roadmap/items/leave-meeting-fails-missing-scope.md` | +3/-2 | Recorded fan-out refusal for next-run dispatch |

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile dated in-progress slices | PASS | PR #524 open; skills run `45156d9b` failed; ToolPolicy run `d72140f4` had not reached the four-hour threshold |
| AC2 | Pick and queue eligible work | PASS | Skills retry and api S3 audit each returned `queued:true` |
| AC3 | Mark queued slices in progress | PASS | Both item front-matters and README rows updated |
| AC4 | Record and verify roadmap changes | PASS | `bash docs/roadmap/lint.sh`: `356 items, 356 index rows, README.md 204 lines — 0 errors, 0 warnings`; `git diff --check` passed |

**Architect handoff:** Keep each queued SDLC slice to its stated scope. The skills retry should inspect the prior developer commit before rebuilding; the api audit must verify masking without reproducing credential values.

**Verdict:** signoff
workflowTriggerParentRunId: 4b8fc765-fefa-4b9b-952e-92d1b9f61856
workflowTriggerRootRunId: 4b8fc765-fefa-4b9b-952e-92d1b9f61856