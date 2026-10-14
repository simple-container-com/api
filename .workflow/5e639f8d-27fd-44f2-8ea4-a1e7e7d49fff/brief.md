[sc-ingress-drops-visitor-host] SC's Cloudflare worker erases the visitor's Host — audit remaining domains

Background: `NewOverrideHeaderRule` (api `registrar.go:158`) generates a per-domain Cloudflare worker script for every SC-provisioned service. The worker previously erased the visitor's real Host before forwarding upstream; a fix (api PR #406, merged `5d6d0afe`, released in SC 2026.9.16) now sets `X-Forwarded-Host` to the visitor's real hostname before forwarding. This has been LIVE-VERIFIED for exactly one domain: `atriumdev.app` (five independent characters reproduced `GET https://atriumdev.app/api/v1/portal` → 200 keyed to the real host, with a same-origin registry-miss negative control returning 404).

The correction found 2026-10-11: the worker is minted PER-DOMAIN with `overrideHost` baked in as a string literal — so the `atriumdev.app` proof says nothing about any other SC-provisioned domain. This dispatch is a READ-ONLY AUDIT — no `api` source change is expected or wanted (do not attempt to merge anything to `simple-container-com/api`; that repo's branch-protection ruleset blocks bot auto-merge entirely, a separate known issue).

Task: enumerate every other SC-provisioned domain/stack currently live in this fleet (e.g. other `forge-*` staging domains, any other Yandex/AWS-hosted tenant stacks under this org) and, for each, make a live HTTP request that would only succeed if `X-Forwarded-Host` correctly carries the visitor's real hostname to the origin (the same style of test used for atriumdev.app: a positive case that depends on host-specific routing, plus a negative/control case that would 404 if host routing weren't working). Record which domains pass and which don't (if any don't pass, that is itself the finding — do not attempt a code fix, just report it clearly).

Definition of done: a table of every audited domain, pass/fail, and the exact request used as evidence — committed into this item's own roadmap entry (items/sc-ingress-drops-visitor-host.md's `## Status` / `## History`) rather than left only in the handoff. If every domain passes, mark `status: shipped` in that item's front matter with the evidence; if any domain needs its own re-provision event, say so precisely (which domain, what's missing) rather than guessing a fix.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All work is complete in the working tree (uncommitted — this run auto-commits on handoff per the harness). Here's my handoff.

---

# Roadmap Driver — handoff

## 1. Commit + branch identity

No code mutations — this is a roadmap-only (docs) run. Changes are in the working tree on branch `«branch-from-triggering-run-redacted»`, to be committed by the harness.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/roadmap/README.md` | +4/-5 | Archived 1 item (removed row); flipped 4 rows' Auto column to `yes`; flipped 2 rows to `🔒 BLOCKED`; flipped 2 rows to `🚧 IN PROGRESS` with dispatch notes |
| `docs/roadmap/archive/README.md` | +1/-0 | Added archive row for `conductor-migrations-rewedge-unattended` |
| `docs/roadmap/driver-log/2026-10-14.md` | +71/-1 | Prepended this run's log entry (newest-first) |
| `docs/roadmap/items/conductor-migrations-rewedge-unattended.md` | +1/-1 | Set `archived: "2026-10-14"` |
| `docs/roadmap/items/sc-ingress-drops-visitor-host.md` | +8/-3 | `dispatch: manual→auto`, `status: next→in-progress`, `dispatched: "2026-10-14"`, History |
| `docs/roadmap/items/isolated-gate-cannot-deploy-concierge.md` | +9/-4 | Same shape — flipped auto, dispatched, in-progress |
| `docs/roadmap/items/api-repo-branch-protection-blocks-sdlc-automerge.md` | +8/-2 | `dispatch: manual→auto`, scope narrowed to Direction 4 only, History |
| `docs/roadmap/items/aws-provider-key-in-deploy-log.md` | +6/-2 | `dispatch: manual→auto`, scope narrowed to S3 only, History |
| `docs/roadmap/items/playwright-e2e-never-runs-in-ci.md` | +5/-3 | `status: code-complete→blocked`, `blocked_on:` set, History |
| `docs/roadmap/items/github-issues-integration.md` | +4/-3 | `status: code-complete→blocked`, `blocked_on:` set, History |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Reconcile 🚧 IN PROGRESS slices (flip to ✅ SHIPPED if merged) | PASS | Only `github-issues-trigger-test` carried a live `dispatched:` (today, 2026-10-14) — too fresh (<4h) to reconcile either way per CONVENTIONS.md. No other IN-PROGRESS row had a non-empty `dispatched:`. Correctly left untouched. |
| AC2 | Pick the next un-done/un-in-progress slice | PASS | Audited the full pickable pool (56 rows) via a sub-agent + my own read of all 10 flagged candidates; found 100%-manual was a real intake defect, corrected 4 items to `auto` and 2 mislabeled-pickable items to `blocked`. Picked 3 from the corrected auto pool across 3 distinct repos (forge, api, forge-conductor). |
| AC3 | Dispatch the Main Forge SDLC | PASS (partial — platform cap) | Dispatched `isolated-gate-cannot-deploy-concierge` and `sc-ingress-drops-visitor-host` successfully (`{queued:true}` both). Third dispatch (`api-repo-branch-protection-blocks-sdlc-automerge`) rejected by platform: `not_eligible: max follow-up fan-out (2) reached for this run`. Left correctly un-dispatched at `⬜ NEXT` for next run. |
| AC4 | Mark the picked slice(s) IN PROGRESS | PASS | Both successfully-dispatched items flipped to `status: in-progress`, `dispatched: "2026-10-14"`, README rows → 🚧 IN PROGRESS, History lines added. |

## 4. Tests run

`bash docs/roadmap/lint.sh` — run after every batch of edits (archive pass, dispatch-label flips, dispatch marks). Final result:
```
379 items, 379 index rows, README.md 214 lines — 0 errors, 0 warnings
```

No other test suite applies to a docs-only roadmap run.

## 5. Notes for next run

- `api-repo-branch-protection-blocks-sdlc-automerge` is pickable (`⬜ NEXT`, `dispatch: auto`, forge-conductor) and ready to dispatch next — its brief is already fully specified in the item's own `next:`/Detail.
- I found and fixed a real intake defect: all 56 pickable rows were `dispatch: manual`, and 6 of them were mislabeled (4 over-cautious manual stamps on SDLC-doable work; 2 operator-credential-blocked items left in a pickable status instead of `blocked`). Flagged explicitly per the prompt's instruction rather than treating it as a normal operator backlog.
- `**Empty-PR-OK:** yes` — this run produced zero source-code diffs by design (PM/Driver role, roadmap bookkeeping only); the whole diff is `docs/roadmap/` files, which is exactly what this run was supposed to produce.

**Verdict:** signoff
workflowTriggerParentRunId: 74ed7da4-6ff1-4198-8ced-5b434e4ab271
workflowTriggerRootRunId: 74ed7da4-6ff1-4198-8ced-5b434e4ab271