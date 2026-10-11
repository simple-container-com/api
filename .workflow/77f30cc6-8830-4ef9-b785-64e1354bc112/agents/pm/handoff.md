[thinking] # PM Handoff — sc-ingress-drops-visitor-host (REDISPATCH, verification-only)

## 0. Restated requirement (from brief)

**This is a verification task, not a code task.** `simple-container-com/api` PR #406 (merge commit `5252819589f8c11f4a4a450c1dbcc4602e1913cb`, head `5d6d0afe`, merged 2026-09-23) added `X-Forwarded-Host` forwarding to SC's shared Cloudflare worker template (`NewOverrideHeaderRule`, `pkg/clouds/pulumi/cloudflare/registrar.go`). It shipped in SC release **2026.9.16** (published 2026-09-30). Nothing here needs new code.

**Job:** find a fleet stack that (a) sits behind this shared Cloudflare worker and (b) has been deployed/redeployed **since** SC 2026.9.16, make a real HTTP request to its public hostname, and produce **server-side evidence** — a log line, debug endpoint, header echo — that the origin received the **visitor's actual Host** via `X-Forwarded-Host`, not the Lambda Function URL's own hostname.

This is a **redispatch**: the previous run (`d1fbcd79`, 2026-10-10) died to four consecutive executor/infra failures (`signal: killed`, `exit status 143` x2, watchdog startup-stall) at the architect→developer handoff. The architect in that run *did* reach a signoff with claimed live evidence, but the run produced **no PR, no commit** — nothing persisted — so that work does not exist and must be redone, not assumed.

## 1. What I confirmed this turn (facts, not yet "the evidence" — that's Architect/Developer's job to produce live)

| Fact | Evidence |
|---|---|
| PR #406 merged | GitHub API: `state=closed`, `merged_at=2026-09-23T19:05:53Z`, `merge_commit_sha=5252819589f8c11f4a4a450c1dbcc4602e1913cb` |
| SC release 2026.9.16 published | GitHub API: tag `2026.9.16`, `published_at=2026-09-30T10:54:52Z` |
| forge-atrium's `internal/portal/portal.go` `Host()` already prefers `X-Forwarded-Host`, strips port/case/trailing dot, handles comma-separated multi-proxy form | Read `internal/portal/portal.go` — doc comment explicitly cites "CF_LAMBDA_URL_DROPS_HOST" and is unit-tested (`portal_test.go`, not re-verified line-by-line this turn) |
| **forge-atrium's `staging` stack (atriumdev.app) has redeployed since the release** | GH Actions: `deploy-forge-atrium.yml` run `37354614013`, commit `0a78ebea4291e1522271c43bfd1c3757ce8c6990`, `conclusion=success`, `created_at=2026-10-05T18:15:44Z` — **after** 2026-09-30. This satisfies "redeployed since 2026.9.16" for forge-atrium. |
| `atriumdev.app` apex is live and serving the Hugo marketing landing | `fetch_url https://atriumdev.app/` → HTTP 200, Hugo-rendered page, confirms DNS/TLS/origin reachable |
| Apex traffic bypasses portal resolution entirely | `cmd/atrium/main.go`'s `apexSet`/`isOwnOrigin` comments: apex hosts are served from embedded assets without ever asking the control plane "is this a portal?" — so hitting the bare apex will **not** exercise the `X-Forwarded-Host`-dependent code path. The evidence must come from a request that triggers `portal.Resolver.Resolve` / `portal.Host(r)` — i.e. a request that depends on which hostname arrived at the origin. |

**Gap I did not close (deliberately — it is Architect's design call, not PM's):** I have not identified a concrete, already-provisioned **tenant portal domain** on `atriumdev.app`'s stack, nor a debug/echo endpoint that surfaces the resolved host back to the caller. Without one of those two, "the origin saw my Host" has no observable signal — the apex page renders identically regardless of what `X-Forwarded-Host` said, because `isOwnOrigin`+apex-match short-circuits before the resolver is ever called.

## 2. Acceptance criteria

| AC# | Criterion | MoSCoW |
|---|---|---|
| AC1 | Identify one SC-deployed fleet stack, redeployed since SC 2026.9.16 (2026-09-30), sitting behind the shared Cloudflare worker | MUST |
| AC2 | Make a real HTTP request to that stack's public hostname (not a unit test, not a template-string check) | MUST |
| AC3 | Capture server-side evidence (log line / debug endpoint / header echo) that the origin received the **visitor's typed hostname** via `X-Forwarded-Host`, distinct from the Lambda Function URL's own hostname | MUST |
| AC4 | If forge-atrium's apex alone cannot exercise the code path (per my finding above), find or construct a request that does — e.g. a registered tenant/preview portal domain on the same stack, or a path that calls `portal.Host(r)`/`portal.Resolver.Resolve` and echoes/logs the result | MUST |
| AC5 | If no such live, exercisable path exists anywhere in the fleet today, say so precisely and name the exact redeploy that would unblock it — do not fabricate a result | MUST (fallback) |
| AC6 | No code changes — this is read + live-request only | MUST (scope guard) |

## 3. Scope decisions

- **MUST**: Live HTTP evidence against a real deployed stack, as scoped above.
- **MUST**: If CloudWatch/Lambda logs are reachable with available credentials, prefer reading an actual access-log line over an engineered debug response — it's closer to "what a visitor actually got."
- **NICE-TO-HAVE**: Checking the `staging-ru` (atriumdev.ru) deployment too, as a second data point — it sits behind YC API Gateway, not Cloudflare, so it's **out of scope** for this specific worker fix (the brief is about the Cloudflare worker specifically). Flag this distinction explicitly if touched, don't conflate the two.
- **CUT**: Re-deriving/re-reading the JS template diff in `registrar.go` — already verified merged and released; re-reading it adds no new evidence toward the live-check ask.
- **CUT**: Attempting to fix or redeploy anything. If the only way to get evidence is a stack that hasn't redeployed, the answer is AC5 (name the blocker), not "redeploy it yourself" — that's an operator/DevOps call, out of a verification turn's scope.

## 4. Handoff guidance for Architect

1. **Start from forge-atrium `staging` (atriumdev.app)** — it's the only fleet stack I found redeployed since 2026.9.16 with `portal.Host()` already reading `X-Forwarded-Host`. Don't re-prove the redeploy timing or the merge/release facts above; take them as given and spend your turn on the live request.
2. **The apex alone won't prove anything** — you need a request that reaches `portal.Resolver.Resolve`/`portal.Host(r)`, not the apex short-circuit. Two realistic paths, in order of preference:
   - Check whether any **tenant portal domain** is already registered in conductor for a stack behind this same worker (search conductor's `portals` collection access, or ask whether a known preview/test domain exists) — then request that domain and compare what the origin logs/echoes for `Host` vs `X-Forwarded-Host`.
   - If no tenant domain exists yet, check whether forge-atrium (or any other candidate stack) exposes **any** endpoint whose response body or logs include the resolved request host (e.g. an error page that names "no portal for host X", a `/healthz` verbose mode, structured JSON access logs via the deployed Lambda's CloudWatch group) — a 404/`ErrNoPortal` response that **names the host it looked up** is still valid evidence, because it shows the origin saw the real hostname rather than the lambda-url hostname.
   - CloudWatch log read (if you hold AWS creds in this run) showing the access log line with `X-Forwarded-Host: <visitor-host>` is the single highest-confidence form of evidence.
3. **If you hit a dead end** — no registered tenant domain, no accessible logs, no echoing surface anywhere in the fleet — do not force a false positive. Report AC5 precisely: name which redeploy (which stack, whose domain, what access you'd need) would make this checkable, and stop there. That is a valid, complete verification-only outcome.
4. **This turn should produce NO code/file changes** in `api`, `forge-atrium`, or elsewhere. If you find yourself wanting to add a debug endpoint to prove this, that is scope creep for a "verification, not code task" brief — flag it as a `record_task`/roadmap note instead, don't implement it.
5. Watch the infra-failure pattern from the last run (`d1fbcd79`): four consecutive `executor_failure`/watchdog kills at the architect→developer boundary. If you hit the same class of error, retry per the retry policy before escalating — but if it recurs identically 4x, report the exact error text and stop; don't silently drop the verification work again.

## 5. Files touched

No code mutations in this turn. (PM plan only.)

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 6. Acceptance criteria verification (PM turn — N/A, this is the plan Architect will execute against)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1–AC6 | (see §2) | PENDING — Architect/Developer turn | Scoped and handed off above; not executable by PM role |

## 7. Tests run

None — PM turn produces no code, so no tests apply.

## Summary

- Confirmed via GitHub API that PR #406 is merged (`5252819589f8c11f4a4a450c1dbcc4602e1913cb`) and SC 2026.9.16 is published (2026-09-30).
- Confirmed forge-atrium's `staging` stack (`atriumdev.app`) deployed **after** that release (GH Actions run `37354614013`, commit `0a78ebea`, 2026-10-05) — the best fleet candidate.
- Confirmed `atriumdev.app`'s apex is live (HTTP 200) but identified a real scoping gap: the apex path bypasses the portal resolver entirely (`isOwnOrigin`/`apexSet` short-circuit in `cmd/atrium/main.go`), so hitting the bare apex produces **no** evidence about `X-Forwarded-Host`. Architect must target a request that actually exercises `portal.Host(r)`/`Resolver.Resolve` — a registered tenant domain, an echoing error path, or CloudWatch access logs.
- This gap, not the release/merge facts, is the real risk to AC3/AC4 — flagged explicitly so Architect doesn't waste the turn re-verifying what's already settled and instead spends it on finding an exercisable code path.
- No code touched; this is pure planning + fact-gathering ahead of the live-request turn.

**Empty-PR-OK:** yes — PM role produces a plan only; no source code is touched or expected to be in this turn.

**Verdict:** signoff