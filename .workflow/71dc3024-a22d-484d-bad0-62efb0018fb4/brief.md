[aws-provider-key-in-deploy-log] SC prints secret-marked provider credentials into the Actions log

A failed isolated deploy (forge run 37628464539, job "Deploy forge-baas (isolated)") printed a live AWS access key AND its secret in plaintext, 4x in one log, as part of a "getResource Invoke RPC finished" diagnostic.

Marking the value secret does not prevent this. The payload shows Pulumi's secret envelope around the value (the 4dabf181.../1b470612... special signatures) — i.e. the property WAS marked secret and Pulumi's own summaries would have redacted it. The leak is in SC itself: api/pkg/clouds/pulumi/events.go:75 (procDiagnosticEvent) forwards evt.DiagnosticEvent.Message verbatim at Info level, and that message is the raw gRPC payload. The exposure is generic — any secret property of any resource or provider, on any cloud, can print this way.

Separately, applyAWSProviderCreds at pkg/clouds/pulumi/aws/provider.go:96-99 passes args.AccessKey/args.SecretKey as plain sdk.String, so the AWS side is unmarked too — but fixing only that would not close this generic path.

## Scope for THIS dispatch — S3 ONLY

Prior phases are already done and are NOT in scope:
- S1 (stop using a human's key for deploys) — SHIPPED 2026-10-07, dedicated user/sc-deploy IAM user is live.
- S2 (revoke the leaked key) — deliberately BLOCKED/operator-owned; do not touch IAM, do not revoke/rotate any credential.

S3 — the only slice to build: redact secret-marked values in procDiagnosticEvent (api/pkg/clouds/pulumi/events.go:75), the one chokepoint every provider's diagnostic messages pass through. Design approach is open to the Architect, but the redaction should be a property of procDiagnosticEvent itself (detecting Pulumi's secret envelope markers in the diagnostic message and masking them), not something each provider has to remember to do individually — per [[pulumi_test_fixture_files_commit_history_hints_e2e_coverage]] and the existing fleet-wide norm of treating credential-material-in-logs as a hard no.

## Definition of done for S3
- A deploy log produced by the current SC binary contains no credential material for any provider (test with a synthetic/fixture secret-marked diagnostic event, not a live credential).
- The redaction lives in procDiagnosticEvent (or a helper it calls), not scattered per-provider.
- Unit tests cover: a secret-marked property's value does not appear in the forwarded log message; a non-secret property's value is unaffected (preserve existing diagnostic usefulness); the Pulumi secret-envelope detection itself (4dabf181.../1b470612... signature recognition).
- Follow fleet logging/redaction conventions already established elsewhere in this codebase (see pkg/api/logger/redact_test.go in the forge-conductor fleet for the kind of test-matrix rigor expected, even though that's a different repo/package — same bar).

## Important operational note — this is a 3rd dispatch attempt
Two prior dispatches of this exact S3 scope both failed in a developer-character executor crashloop (exit status 143 / signal killed, "claude emitted no output beyond init for 5m0s") before any code was written or any PR opened — this looks like infra flake (hung tool/MCP handshake at turn startup) rather than a content problem, since the Architect's design handoff was clean both times. If you are the Developer character and hit a similar hang, that is a known pattern on this specific slice/repo combination — report it plainly rather than guessing at a workaround.

## Side-effect this run also serves (do not treat as primary scope)
This dispatch targets api, which doubles as the live proof for a different roadmap item, api-repo-branch-protection-blocks-sdlc-automerge (api's branch protection requires 2 human reviews, so the SDLC bot cannot self-merge there — a forge-conductor fix for this already shipped and is deployed). If your PR against api completes without an auto-merge-escalation (either it merges cleanly, or it ends as a normal non-escalated PR awaiting human review rather than an alarming "auto-merge did not land" escalation), note that explicitly in your handoff (PR number, mergeable_state). This is observational only — do not scope any code changes to it.

## Secret hygiene
Do not echo any real credential value anywhere in commits, tests, or handoffs — use synthetic/fixture values only, even though this item IS about a credential leak.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All changes look correct and intentional. Handoff follows.

---

# Roadmap Driver — Handoff

## 1. Commit + branch identity

No code mutations in this turn — this is a Roadmap Driver character; my edits are to `docs/roadmap/` files in this umbrella repo's own working tree, which the harness commits automatically at end of turn on branch `«branch-from-triggering-run-redacted»`.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +4/-2 | Flipped `api-repo-branch-protection-blocks-sdlc-automerge` row to 🟡 CODE-COMPLETE with corrected gate text; flipped `aws-provider-key-in-deploy-log` row to 🚧 IN PROGRESS (re-dispatched) |
| docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md | +27/-10 | `status: in-progress → code-complete`; corrected two stale identifiers (commit SHA, deploy run id) per DevOps's own verified evidence; added Status-table rows for negative-control proof + still-open positive-path proof; History entry |
| docs/roadmap/items/aws-provider-key-in-deploy-log.md | +24/-3 | Reconciled 2nd crashloop failure → `redispatch`, then re-picked → `status: in-progress`, `dispatched: "2026-10-16"`; two History entries (reconcile + re-dispatch) |
| docs/roadmap/driver-log/2026-10-16.md | new file | Run note: intake/reconcile/archive/pick/dispatch/mark/escalate/verify, with GitHub evidence checked |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile 🚧 IN PROGRESS slices; flip to ✅ SHIPPED if SDLC PR merged | PASS | Checked both items with `dispatched:` set. `api-repo-branch-protection-blocks-sdlc-automerge`: PR #587 merged but DoD's positive-path clause remains open → `code-complete`, not `shipped`. `aws-provider-key-in-deploy-log`: run `3ee0dba1` failed (developer-character crashloop, no PR/commit) → `redispatch` |
| AC2 | Pick the next un-done/un-in-progress slice | PASS | Pickable pool after reconcile: `aws-provider-key-in-deploy-log` (⬜ REDISPATCH, auto) and `api-repo-branch-protection-blocks-sdlc-automerge` (🟡 CODE-COMPLETE, auto, but its only gap is an `api`-dispatch side-effect, not independently re-dispatchable). Picked the former |
| AC3 | Dispatch the Main Forge SDLC | PASS | `trigger_workflow` → workflow `99171cbe-7c27-4976-a999-931b834b5d62`, `linked_repo_id 6a1df1a2e639099d50d23c32` (api) → `{queued:true}` |
| AC4 | Mark the picked slice IN PROGRESS | PASS | `items/aws-provider-key-in-deploy-log.md`: `status: in-progress`, `dispatched: "2026-10-16"`, History line; README row flipped to 🚧 IN PROGRESS |

## 4. Tests run

N/A — no code changes. Verification command run: `bash docs/roadmap/lint.sh` → `393 items, 393 index rows, README.md 230 lines — 0 errors, 0 warnings` (run twice, before and after edits).

## 5. Notable findings this run

- **Stale-item escalation**: `aws-provider-key-in-deploy-log`'s S3 dispatch has now failed twice in a row with the identical developer-character crashloop signature (`exit status 143`, hung tool/MCP handshake at turn startup) before any code landed. Flagged explicitly in the item's `next:` and in the new dispatch's brief for whoever reads it next — if a third attempt also crashes, this is likely an infra pattern specific to this slice/repo rather than noise. Sent a Telegram escalation per STEP 5b.
- **Record correction**: `api-repo-branch-protection-blocks-sdlc-automerge`'s DevOps turn (PR #587) caught that four prior characters had cited the wrong commit SHA (`00c06a29`, a pre-squash SHA unreachable from any deploy) and the wrong deploy run (`38674922361`, which is actually PR #585's unrelated deploy). Corrected both in the item file with the DevOps-verified values (`«sha-from-triggering-run-redacted»`, deploy run `38665872779`).
- No stray Inbox entries, no archive candidates (all shipped items either carry an open marker or are within the 2-day freshness window).

**Empty-PR-OK:** yes — this run's diff is entirely `docs/roadmap/` index/item/log edits (reconcile + dispatch-mark + run log), the designated output of the Roadmap Driver role, not `.workflow/` engine metadata.

**Verdict:** signoff
workflowTriggerParentRunId: c252f036-b7f7-4809-b8a8-a05816458eb2
workflowTriggerRootRunId: c252f036-b7f7-4809-b8a8-a05816458eb2