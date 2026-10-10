[sc-ingress-drops-visitor-host] SC's Cloudflare worker erases the visitor's Host

Context: simple-container-com/api PR #406 (merged 2026-09-23, commit 52528195) fixed SC's shared Cloudflare worker template to forward the visitor's original Host as X-Forwarded-Host (previously the worker overwrote Host for the upstream fetch and nothing told the origin which domain was actually requested). This shipped in SC release 2026.9.16 (published 2026-09-30). What has never been checked is whether a visitor's Host now actually arrives at a stack that has been redeployed since that release — a live check, not a unit test of the template string.

Task: this is verification-only — do NOT write new source code unless you find and must fix a genuine regression.
1. Identify a Forge-fleet stack that has been redeployed (new SC release picked up) since 2026-09-30 and that sits behind the shared Cloudflare worker — forge-atrium (atriumdev.app / atriumdev.ru) is the likeliest candidate since it is the one service in the fleet that routes by host and depends on this header (see forge-atrium's `internal/portal.Host`, which already reads X-Forwarded-Host).
2. Make a real HTTP request to that stack with a distinguishing Host header (or just use its real public hostname) and confirm, from server-side evidence (a log line, an API response that echoes/depends on the resolved host, or equivalent), that the visitor's actual Host reached the origin — not the Lambda Function URL.
3. If no redeployed stack is reachable for a live check, say so precisely and name exactly what redeploying would require.

Definition of done: a live request against a real deployed stack proves X-Forwarded-Host (or an equivalent host-derived behavior) carries the visitor's real hostname — recorded with the actual request/response evidence, not inferred from a green build.

Do not edit docs/roadmap/ in the forge repo from this dispatch — that bookkeeping belongs to the Roadmap Driver.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All work is in place. Here's the handoff.

# Roadmap Driver handoff — 2026-10-10 (run 3)

## 1. Commit + branch identity
No code mutations — this is a pure roadmap/docs turn. Edits are committed by the engine on this run's branch.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +2/-2 | Flipped `no-oneclickpath-to-connect-project` → `🔒 BLOCKED`, `meeting-agent-goes-silent-mid-conversation` → `🟡 CODE-COMPLETE`, then flipped `sdlc-verification-only-runs-fail-substance-gate` + `sc-ingress-drops-visitor-host` → `🚧 IN PROGRESS` (newly dispatched) |
| docs/roadmap/items/meeting-agent-goes-silent-mid-conversation.md | +23/-10 | `status: in-progress`→`code-complete`, `dispatch: auto`→`manual`; History entry with root-cause + shipped-fix evidence and the two operator-only gates (audibility, latency log access) |
| docs/roadmap/items/no-oneclickpath-to-connect-project.md | +22/-11 | `status: in-progress`→`blocked`, `dispatch: auto`→`manual`; History entry recording the real `Verdict: failure` and the no-eligible-repo blocker |
| docs/roadmap/items/sdlc-verification-only-runs-fail-substance-gate.md | +9/-5 | `status: code-complete`→`in-progress`, `dispatched: "2026-10-10"`; History entry for dispatch |
| docs/roadmap/items/sc-ingress-drops-visitor-host.md | +8/-5 | `status: code-complete`→`in-progress`, `dispatched: "2026-10-10"`; History entry for dispatch |
| docs/roadmap/driver-log/2026-10-10.md | +53/-0 | Appended this run's note (newest-first) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile prior 🚧 IN PROGRESS dispatches | PASS | Both `dispatched: "2026-10-10"` rows checked against their actual run handoffs (not just PR-merged state): `meeting-agent-goes-silent-mid-conversation` (PR #561 merged, but DevOps's own verdict is code-complete not shipped) and `no-oneclickpath-to-connect-project` (PR #559 merged, but DevOps's own verdict is `Verdict: failure`) |
| AC2 | Archive gate | PASS | Checked both shipped zero-marker candidates — both `updated: 2026-10-09`, one day short of the ≥2-day gate. No archive pass, correctly |
| AC3 | Pick + dispatch up to 4 slices | PASS (partial, platform-capped) | 3 eligible auto/non-manual CODE-COMPLETE rows found; 2 of 3 dispatched (`queued:true` both), 3rd blocked by platform fan-out cap (2), left for next run |
| AC4 | Mark dispatched slices IN PROGRESS | PASS | Both item files + README rows updated with `dispatched: "2026-10-10"` |
| AC5 | lint.sh passes | PASS | `366 items, 366 index rows, README.md 209 lines — 0 errors, 0 warnings` |

## 4. Tests run
`bash docs/roadmap/lint.sh` → `0 errors, 0 warnings` (one stale `design:` path and one over-length `summary:` were caught and fixed before the clean run).

## Summary of this run
- **Reconciled** both in-flight dispatches by reading the actual DevOps handoff verdicts, not the engine's `completed` status alone: `meeting-agent-goes-silent-mid-conversation` shipped a real root-cause fix (SSE stall-cue) but is **code-complete**, gated on two operator-only steps (live-call audibility check; log/DB grant for the underlying 8–13s latency, filed as forge-conductor issue #560). `no-oneclickpath-to-connect-project` actually reported `Verdict: failure` — the idempotent-connect path is proven live on prod, but the first-create path has **zero eligible targets** in the org (every unregistered repo is archived), so it's now **blocked** on an operator naming a disposable repo.
- **Dispatched** 2 of 3 eligible CODE-COMPLETE slices (`sdlc-verification-only-runs-fail-substance-gate`, `sc-ingress-drops-visitor-host`); the 3rd (`google-meet-speaker-names-not-surfaced-in-turn`) is queued for the next run, blocked only by the platform's 2-per-run fan-out cap.
- No escalations needed this run — no item flipped to `redispatch`.

**Empty-PR-OK:** yes — this is a pure roadmap-bookkeeping turn (reconcile + pick + dispatch), no source code was touched or expected to be.

**Verdict:** signoff
workflowTriggerParentRunId: cfccedaf-e5d9-49f1-9e23-1f2a581d05af
workflowTriggerRootRunId: cfccedaf-e5d9-49f1-9e23-1f2a581d05af