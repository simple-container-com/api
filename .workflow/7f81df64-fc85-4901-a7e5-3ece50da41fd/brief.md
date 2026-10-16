[aws-provider-key-in-deploy-log] SC prints secret-marked provider credentials into the Actions log

**4th dispatch attempt on this slice. READ THIS FIRST.** Three prior dispatches (runs `b0de022e`, `3ee0dba1`, `71dc3024`) all died in a Developer-character executor crashloop — repeated `exit status 143` over the Developer's turn, with NO PR opened and NO commit landed, despite the Architect handoff being clean (signoff) each time. This is not a content problem; it is specifically the Developer character's executor failing on this repo/slice combination. If you are the Developer character on this run and you sense anything unusual about turn startup (hung MCP handshake, slow git clone, unusual workspace state), consider that pattern before assuming it's a one-off — three in a row on the same slice is no longer simple noise.

**Scope: S3 only.** Redact Pulumi's raw diagnostic-event payload in `procDiagnosticEvent` (`api/pkg/clouds/pulumi/events.go:75`). The payload forwards `evt.DiagnosticEvent.Message` verbatim at Info level, and that message is Pulumi's raw gRPC `getResource` response — including secret-marked provider credentials (AWS access key + secret key were leaked this way in forge run 37628464539). This is the one chokepoint every provider's diagnostics pass through, so fixing it here closes the leak path generically (not just for AWS, not just for one provider).

**Do NOT touch:**
- S1 (already shipped — the leaked key's IAM user was replaced with a dedicated `sc-deploy` user)
- S2 (revoking the leaked key — this is a deliberate operator judgement call about cutting a live fleet credential; explicitly out of scope)
- `api/pkg/clouds/pulumi/aws/provider.go:96-99` (the AWS-specific plain-string issue) — that's a different, narrower fix; this slice is about the generic `procDiagnosticEvent` chokepoint

**Definition of done:**
- A deploy log produced by a current SC binary contains no credential material for any provider's secret-marked properties, even when a diagnostic event carries them.
- The redaction is a property of `procDiagnosticEvent` itself (or something it calls), not of each provider remembering to mark its own inputs correctly.
- Tests cover at least: a diagnostic message containing a Pulumi secret-envelope-wrapped value gets redacted; a diagnostic message with no secret material passes through unchanged.

**Context for the Architect:** this repo (`api`, simple-container-com/api) is public/open-source with branch protection requiring 2 human reviews — PRs here do not auto-merge via the SDLC bot (see roadmap item `api-repo-branch-protection-blocks-sdlc-automerge`, now code-complete). Expect your PR to land `mergeable_state: blocked` and require an operator to merge by hand; that is normal for this repo and not a sign anything went wrong. Do not expand scope to try to fix that — it's a separate, already-addressed item. If your PR actually opens successfully against `api`, note your PR number and `mergeable_state` in your handoff — it would be the first live proof since the branch-protection fix deployed, which that other roadmap item is still waiting on.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
Lint clean, 0 errors. Work complete.

---

# Handoff — Roadmap Driver run a72621eb-3dc8-4d9a-8e9d-e3cee7be1109

## 1. Commit + branch identity
No code mutations — this is a roadmap-index-only turn. Files are left in the working tree for the engine's auto-commit/merge; I did not run `git commit`/`git push` myself (git auth isn't configured in this sandbox and the harness commits automatically per character-turn convention).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +2/-5 (net) | Removed 4 archived rows; flipped `aws-provider-key-in-deploy-log` row ⬜REDISPATCH→🚧IN PROGRESS with updated gate text |
| docs/roadmap/archive/README.md | +4/-0 | Added archive rows for 4 newly-archived items |
| docs/roadmap/items/aws-provider-key-in-deploy-log.md | +12/-2 | Reconciled 3rd failed dispatch → `redispatch`, then re-dispatched 4th attempt → `in-progress`, `dispatched: "2026-10-16"`, two new History lines |
| docs/roadmap/items/isolated-gate-cannot-deploy-concierge.md | +1/-1 | `archived: 2026-10-16` set |
| docs/roadmap/items/provider-activate-deactivates-fleet.md | +1/-1 | `archived: 2026-10-16` set |
| docs/roadmap/items/browse-seeds-every-org-secret-on-chat-turns.md | +1/-1 | `archived: 2026-10-16` set |
| docs/roadmap/items/cloudexec-turns-degrade-silently-without-shell.md | +1/-1 | `archived: 2026-10-16` set |
| docs/roadmap/driver-log/2026-10-16.md | +52/-0 | Prepended this run's note (newest-first) above the prior run's entry |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile any 🚧 IN PROGRESS slices (flip to ✅ SHIPPED if SDLC PR merged) | PASS | Only one in-progress row carried a live `dispatched:` (`aws-provider-key-in-deploy-log`). Checked `list_runs(sliceId=…)` → run `71dc3024` `status:failed`, checked `list_pull_requests`(api, state=all) + `list_repository_commits`(events.go) → no PR, no commit. Flipped to `redispatch` (3rd consecutive crashloop, not a ship). |
| AC2 | Pick the next un-done/un-in-progress slice | PASS | Scanned full pickable pool (`⬜ REDISPATCH`→`⬜ NEXT`→`🟡 CODE-COMPLETE`, `dispatch: auto` only) via `grep -vE '**MANUAL**'` across all 60 pickable rows — only 2 auto rows exist; picked `aws-provider-key-in-deploy-log` (REDISPATCH, higher priority than the other auto candidate which has no independent work left per its own item history). |
| AC3 | Dispatch the Main Forge SDLC | PASS | `trigger_workflow` → workflow `99171cbe-7c27-4976-a999-931b834b5d62`, `linked_repo_id: 6a1df1a2e639099d50d23c32` (api) → `{queued:true}` |
| AC4 | Mark the picked slice IN PROGRESS | PASS | Item file: `status: in-progress`, `dispatched: "2026-10-16"`, History line added. README row flipped to 🚧 IN PROGRESS. |

## 4. Tests run
`bash docs/roadmap/lint.sh` → `393 items, 393 index rows, README.md 226 lines — 0 errors, 0 warnings` (run twice, before and after edits — both clean).

## 5. Summary of judgment calls
- **Archive pass (STEP 2):** 4 items (`isolated-gate-cannot-deploy-concierge`, `provider-activate-deactivates-fleet`, `browse-seeds-every-org-secret-on-chat-turns`, `cloudexec-turns-degrade-silently-without-shell`) cleared the 2-day freshness gate today that blocked them in yesterday's run (all `updated: 2026-10-14`). Archived per CONVENTIONS.md mechanics.
- **Dispatch cap:** Only 1 of 4 available slots used — the only other `dispatch: auto` candidate (`api-repo-branch-protection-blocks-sdlc-automerge`) has no independent work left; its remaining DoD clause is a live-proof side-effect of an `api`-repo bot PR landing, which this run's dispatch is itself the opportunity for.
- **Escalation sent** via Telegram for the 3-consecutive-crashloop pattern on `aws-provider-key-in-deploy-log`, per STEP 5b.

**Empty-PR-OK:** yes — this is the Roadmap Driver's normal turn shape: roadmap-index bookkeeping (reconcile/archive/dispatch) with no source-code changes, consistent with every prior Driver run in this repo's history.

**Verdict:** signoff
workflowTriggerParentRunId: a72621eb-3dc8-4d9a-8e9d-e3cee7be1109
workflowTriggerRootRunId: a72621eb-3dc8-4d9a-8e9d-e3cee7be1109