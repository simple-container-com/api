[aws-provider-key-in-deploy-log] SC prints secret-marked provider credentials into the Actions log

A failed isolated deploy (`forge` run 37628464539, job "Deploy forge-baas (isolated)") printed a live AWS access key AND its secret in plaintext, 4x in one log, as part of a `getResource Invoke RPC finished` diagnostic.

Marking the value secret does not prevent this. The payload shows Pulumi's secret envelope around the value (the `4dabf181…` / `1b470612…` special signatures), i.e. the property WAS marked and Pulumi's own summaries would have redacted it. The leak is in SC itself: `api/pkg/clouds/pulumi/events.go:75` (`procDiagnosticEvent`) forwards `evt.DiagnosticEvent.Message` verbatim at Info level, and that message is the raw gRPC payload. The exposure is generic — any secret property of any resource or provider, on any cloud, can print this way.

Separately, `applyAWSProviderCreds` at `pkg/clouds/pulumi/aws/provider.go:96-99` passes `args.AccessKey`/`args.SecretKey` as plain `sdk.String` (unmarked) — but fixing only that would not close the generic path through `procDiagnosticEvent`.

## Scope for THIS dispatch (S3 only)

Fix `procDiagnosticEvent` (`api/pkg/clouds/pulumi/events.go:75`) to redact secret material before forwarding `evt.DiagnosticEvent.Message` to the log — this is the one chokepoint every provider's diagnostics pass through. Do NOT touch S1 (already done — deploys now use a dedicated `sc-deploy` IAM user) or S2 (revoking the old leaked key — that is an explicit operator judgement call, out of scope here).

## Definition of done

- A deploy log from a current SC binary contains no credential material for any provider, even when a diagnostic event's payload carries Pulumi's secret envelope around a plaintext value.
- The redaction is a property of `procDiagnosticEvent` itself (the chokepoint), not of each provider remembering to mark its inputs — so it protects AWS, GCP, Yandex, and any future provider uniformly.
- Add/extend unit tests covering: a diagnostic message containing Pulumi's secret-envelope signature is redacted; a message with no secret envelope passes through unchanged; test against a real sample shape of the `getResource Invoke RPC finished` message that leaked.
- Full test suite for the touched package passes; `go build ./...` and `go vet ./...` succeed.
- Note: this repo (`api`) now has a working auto-merge path for SDLC dispatches (api-repo-branch-protection-blocks-sdlc-automerge, fixed 2026-10-15, commit 00c06a29) — this dispatch doubles as the first live proof of that fix. Expect a normal PR that gets evaluated for auto-merge without an escalation purely due to branch-protection review requirements.

Note: this is a RE-DISPATCH. A prior attempt (run b0de022e, 2026-10-15) died to an executor crashloop (watchdog timeout, no output beyond init) during the Developer turn — infra flake, not a content defect. The Architect's design handoff before the crash signed off cleanly; feel free to re-derive the implementation approach from scratch.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
These edits are uncommitted working-tree changes that will be committed automatically at the end of this turn per the run's auto-commit mechanism (consistent with this repo's workflow pattern). Here's my handoff.

---

# Roadmap Driver — Handoff

## 1. Commit + branch identity

No code mutations in this turn — this is a Roadmap Driver character; my edits are to `docs/roadmap/` files in this umbrella repo's own working tree, which the harness commits automatically at end of turn on branch `«branch-from-triggering-run-redacted»`.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/roadmap/items/aws-provider-key-in-deploy-log.md` | +14/-7 | Reconciled failed dispatch (`in-progress`→`redispatch`→`in-progress` after re-dispatch); 2 new `## History` lines |
| `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +12/-5 | `code-complete`→`in-progress` after re-dispatch for live-validation proof; 1 new `## History` line |
| `docs/roadmap/README.md` | +4/-2 | Index rows for both items updated (status cell + next-step text) |
| `docs/roadmap/driver-log/2026-10-15.md` | +76 | Appended this run's note (newest-first) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | STEP 0: intake stray Inbox entries | PASS | `awk '/## Inbox/{flag=1} flag' README.md` → section empty, nothing to file |
| AC2 | STEP 1: reconcile 🚧 IN PROGRESS rows with `dispatched:` set | PASS | Found 1: `aws-provider-key-in-deploy-log`. `list_runs(sliceId=..., includeTerminal=true)` → run `b0de022e`, `status: failed`, dispatched 08:34Z, last activity 11:12Z (>4h before now 16:24Z). `get_run` timeline shows Developer-character executor crashloop (3× exit 143 / watchdog timeout), no PR, no commit. `list_pull_requests` on `api` confirms zero hits for the `[aws-provider-key-in-deploy-log]` token. Flipped to `redispatch` |
| AC3 | STEP 2: archive gate check | PASS | Checked all shipped candidates; newest ship dates are 2026-10-14/15 (0-1 days old), fail the ≥2-day gate. No archive pass — correctly declined |
| AC4 | STEP 3: pick up to 4 pickable+auto slices | PASS | Programmatic scan of all 3 horizon tables for `⬜ REDISPATCH`/`⬜ NEXT`/`🟡 CODE-COMPLETE` with Auto≠MANUAL found exactly 2: `aws-provider-key-in-deploy-log` (redispatch, api) and `api-repo-branch-protection-blocks-sdlc-automerge` (code-complete, forge-conductor). Both picked, different repos |
| AC5 | STEP 4: dispatch via `trigger_workflow` with correct `linked_repo_id` | PASS | `api`→`6a1df1a2e639099d50d23c32`, `forge-conductor`→`6a1d87679aa5469478145de4` — both matched the dispatch table exactly, both `{queued:true}` |
| AC6 | STEP 5: mark dispatched slices IN PROGRESS (item + index) | PASS | Both item files: `status: in-progress`, `dispatched: "2026-10-15"`, `updated: 2026-10-15`, `## History` line added. Both README rows flipped to `🚧 IN PROGRESS` |
| AC7 | STEP 5b: escalate the redispatched slice | PASS | `schedule_notification` with `send_now: true` → `{sent:true}` |
| AC8 | STEP 6: log the run | PASS | `docs/roadmap/driver-log/2026-10-15.md` — newest-first entry prepended, covers all steps + GitHub evidence checked |
| AC9 | STEP 7: lint must exit 0 errors | PASS | `bash docs/roadmap/lint.sh` → `393 items, 393 index rows, README.md 230 lines — 0 errors, 0 warnings` |

## 4. Tests run

Not applicable — doc-only roadmap edit, no source code touched. Verification was `bash docs/roadmap/lint.sh` (PASS, shown above), plus live GitHub API checks (`list_runs`, `list_pull_requests`, `get_run`) to confirm the failed-dispatch diagnosis before reconciling.

## 5. Verdict line

**Empty-PR-OK:** yes — this is a roadmap-only reconcile/pick/dispatch turn; no source code changes are expected or possible for this character, and the AC table above carries real live evidence (run ids, PR-search results, lint output) rather than narrative claims.

**Verdict:** signoff
workflowTriggerParentRunId: 5bef2145-2cfd-4b58-bb7d-33e95d3b66ee
workflowTriggerRootRunId: 5bef2145-2cfd-4b58-bb7d-33e95d3b66ee