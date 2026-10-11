[sc-ingress-drops-visitor-host] SC's Cloudflare worker erases the visitor's Host

Context: this is a REDISPATCH — the prior run (d1fbcd79, 2026-10-10) died to repeated executor/infra failures (watchdog timeouts, signal: killed) before any character could act; it produced no PR and no commit. The underlying work was already correctly scoped by that run's architect (who did sign off with real evidence before the chain died downstream) — re-attempt it fresh.

Background: api PR #406 (commit 5d6d0afe, merged 2026-09-23) added X-Forwarded-Host forwarding to SC's shared Cloudflare worker template (NewOverrideHeaderRule in registrar.go). It shipped in SC release 2026.9.16 (published 2026-09-30). This is a VERIFICATION task, not a code task — the fix is already merged and released.

Task: find a fleet stack that has been deployed/redeployed since SC 2026.9.16 and sits behind this shared Cloudflare worker (forge-atrium is the likeliest candidate — its internal/portal.Host code already reads X-Forwarded-Host and is tested). Make a real HTTP request to that stack's public hostname and get server-side evidence (a log line, a debug endpoint, a header echo, etc. — not just "it didn't error") that the ORIGIN received the visitor's actual Host header via X-Forwarded-Host, rather than seeing the Lambda Function URL's own hostname.

Definition of done: X-Forwarded-Host arrives at a deployed SC service carrying the hostname the visitor typed, verified against a real request — not a unit test of the template string. If no suitable redeployed stack exists, say so precisely and name what redeploy would unblock it, rather than fabricating a result.

Do NOT edit docs/roadmap/ in the forge repo from this dispatch — that bookkeeping belongs to the Roadmap Driver.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All changes confirmed. Here's my handoff.

---

# Roadmap Driver handoff — 2026-10-11

## 1. Commit + branch identity
No code mutations — this is a pure roadmap/docs turn. Edits are committed by the engine on this run's branch (`«branch-from-triggering-run-redacted»`).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| docs/roadmap/README.md | +8/-10 | Removed archived rows (`ktalk-diarization-missing`, `meeting-agents-no-org-wide-recall-grant`); flipped `sdlc-verification-only-runs-fail-substance-gate` → `✅ SHIPPED`; flipped `sc-ingress-drops-visitor-host` → `⬜ REDISPATCH` then `🚧 IN PROGRESS`; flipped `google-meet-speaker-names-not-surfaced-in-turn` → `🚧 IN PROGRESS` |
| docs/roadmap/archive/README.md | +2/-0 | Added archive rows for the two newly-archived items |
| docs/roadmap/items/sdlc-verification-only-runs-fail-substance-gate.md | +9/-9 | `status: in-progress`→`shipped`; Status table's behavioural-smoke row closed out; History entry |
| docs/roadmap/items/sc-ingress-drops-visitor-host.md | +18/-6 | `status: in-progress`→`redispatch`→`in-progress` (re-dispatched same run); `dispatched:` cleared then reset to `"2026-10-11"`; two History entries (failure reconcile + redispatch) |
| docs/roadmap/items/google-meet-speaker-names-not-surfaced-in-turn.md | +8/-6 | `status: code-complete`→`in-progress`, `dispatched: "2026-10-11"`; History entry |
| docs/roadmap/items/ktalk-diarization-missing.md | +1/-1 | `archived: ""` → `archived: "2026-10-11"` |
| docs/roadmap/items/meeting-agents-no-org-wide-recall-grant.md | +1/-1 | `archived: ""` → `archived: "2026-10-11"` |
| docs/roadmap/driver-log/2026-10-11.md | +52/-0 | New file, this run's note |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile prior 🚧 IN PROGRESS dispatches | PASS | Checked both `dispatched: "2026-10-10"` rows against actual GitHub/run state, not engine status alone: `sdlc-verification-only-runs-fail-substance-gate` → run `e138415e` completed, PR #565 merged `e60052fae2`, confirmed via `read_pull_request_diff` + `get_run` → `shipped`. `sc-ingress-drops-visitor-host` → run `d1fbcd79` failed (4+ consecutive executor errors, 4h+ stuck, zero PRs found via `list_pull_requests`) → `redispatch` |
| AC2 | Archive gate | PASS | `ktalk-diarization-missing` + `meeting-agents-no-org-wide-recall-grant` both `shipped`, zero outstanding markers (verified programmatically), `updated: 2026-10-09` = exactly 2 days old = gate open. Archived both |
| AC3 | Pick + dispatch up to 4 slices | PASS | Pool computed off index table = exactly 2 eligible rows (`sc-ingress-drops-visitor-host` REDISPATCH, `google-meet-speaker-names-not-surfaced-in-turn` CODE-COMPLETE). Both dispatched, both `queued:true`, no fan-out cap encountered |
| AC4 | Mark dispatched slices IN PROGRESS | PASS | Both item files + README rows updated with `dispatched: "2026-10-11"` |
| AC5 | Escalate stale/redispatch items | PASS | `schedule_notification` sent immediately for `sc-ingress-drops-visitor-host` before STEP 6, per STEP 5b |
| AC6 | lint.sh passes | PASS | `366 items, 366 index rows, README.md 207 lines — 0 errors, 0 warnings` |

## 4. Tests run
`bash docs/roadmap/lint.sh` → `366 items, 366 index rows, README.md 207 lines — 0 errors, 0 warnings`

## Summary
- **Reconciled**: `sdlc-verification-only-runs-fail-substance-gate` shipped cleanly — its own dispatch was the behavioural smoke it needed, and it passed (PR #565 merged, Empty-PR-OK bypass fired as designed). `sc-ingress-drops-visitor-host` failed to infra errors (watchdog/signal-killed, never reached a content verdict) — escalated to Telegram and immediately redispatched.
- **Archived**: `ktalk-diarization-missing`, `meeting-agents-no-org-wide-recall-grant` — both cleared the 2-day gate with zero outstanding work.
- **Dispatched**: `sc-ingress-drops-visitor-host` (redispatch, api) and `google-meet-speaker-names-not-surfaced-in-turn` (code-complete, forge-conductor) — both `queued:true`, no fan-out cap hit (pool was only 2 deep).

**Empty-PR-OK:** yes — this is a pure roadmap-bookkeeping turn (reconcile + archive + pick + dispatch), no source code was touched or expected to be.

**Verdict:** signoff
workflowTriggerParentRunId: 2b40d5ac-2782-4f09-9f0c-5109bf2c3e62
workflowTriggerRootRunId: 2b40d5ac-2782-4f09-9f0c-5109bf2c3e62