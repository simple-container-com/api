[thinking] # Architect Handoff — sc-ingress-drops-visitor-host (verification-only)

## 0. Scope restated
Confirm that SC's shared Cloudflare worker template (fixed in `simple-container-com/api` PR #406, commit `5252819589f8c11f4a4a450c1dbcc4602e1913cb`, merged 2026-09-23, released as SC 2026.9.16 on 2026-09-30) actually delivers the visitor's real `Host` as `X-Forwarded-Host` to a redeployed fleet stack — live, not inferred. My job here is to audit PM's prior two (identical) handoffs, independently verify the chain from worker script → consuming code → live HTTP response, and close or redirect the item.

## 1. Commit + branch identity
No code mutations in this turn — Architect role, verification/audit only. Repo is reachable (not inaccessible), so this is a confirmed (not provisional) design/verification note.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none — verification-only turn, no source edited in `api` or `forge-atrium` |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Identify a Forge-fleet stack redeployed since 2026-09-30, sitting behind the shared CF worker | PASS | **forge-atrium** (`atriumdev.app`). Confirmed independently, not just via PM's claim: `read_repository_file` on `.github/workflows/deploy-forge-atrium.yml` shows `on: push: branches: [main]` triggers `deploy-client-stack@main`, which runs `docker://simplecontainer/github-actions:latest` — i.e. **every deploy pulls the current SC release**, so any push-triggered deploy after 2026-09-30 necessarily runs on SC ≥2026.9.16. I then queried the GitHub Actions REST API directly (`GET /repos/simple-container-com/forge-atrium/actions/workflows/deploy-forge-atrium.yml/runs`) and found run **id 37354614013**, `head_sha=0a78ebea4291e1522271c43bfd1c3757ce8c6990` (2026-10-05T18:15:36Z commit), `status:"completed"`, `conclusion:"success"` — a **real, completed, successful redeploy** 5 days after the release, not just a commit sitting on `main`. `.sc/stacks/forge-atrium/client.yaml` confirms `template: lambda` + `domain: atriumdev.app` (Lambda Function URL behind a Cloudflare apex CNAME) — the exact `ProvisionDomainForEndpoint` → `NewOverrideHeaderRule` code path PR #406 touched (confirmed by reading `pkg/clouds/pulumi/cloudflare/registrar.go` directly, SHA `c9929918b51225e80d96f8692b19efa441fe872a` — contains the `fwdHeaders.set("X-Forwarded-Host", origHost)` line verbatim). |
| AC2 | Make a real HTTP request with a distinguishing Host and confirm server-side evidence the visitor's real Host reached the origin (not the Lambda Function URL) | PASS | Live, independently re-run in this turn: `GET https://atriumdev.app/api/v1/portal` → `HTTP 200`, body `{"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},"blocks":["sign-in","chat","processes"],"assistantName":"Atrium Assistant","chatTransport":"sse"}`, response headers include `Server: cloudflare`, `Cf-Ray: a4872f2a195f2bfa-FRA`, `X-Amzn-Requestid: 5a56521e-d727-4b03-83b8-d6a2f1dc75b5` (proves the request transited Cloudflare *and* reached the Lambda — both hops confirmed in one response). `GET https://atriumdev.ru/api/v1/portal` → `HTTP 200`, same brand/blocks, `"chatTransport":"websocket"`, `Server: ycalb` (no `Cf-Ray` — correctly, since `.ru` is YC API Gateway, not this CF worker — matches `client.yaml`'s own design comment). **Chain of proof, read from the actual source this turn:** `internal/portal/portal.go`'s `Host()` function (read verbatim, SHA `e12c6b8c57487dd86256a439d97ec92f9fcc3eab`) reads `r.Header.Get("X-Forwarded-Host")` first, falls back to `r.Host` only if empty — with its own doc comment naming the exact failure mode this item is about ("a Lambda URL behind Cloudflare receives the function's own hostname in Host, not the customer's"). `internal/blocks/blocks.go`'s `Portal` handler calls `h.portalOr404` → `portal.Resolver.Resolve(ctx, portal.Host(r))`, which returns `ErrNoPortal` (→ 404 JSON) for any host not registered as a portal (confirmed in `portal.go`: `Resolve` on `ErrNoPortal` from `Source.LookupPortal`, caches negative, returns 404-triggering sentinel). Independently confirmed via DNS (`dns.google` DoH) that `atriumdev.app` resolves to Cloudflare anycast IPs (`172.67.148.148`, `104.21.29.73`) — i.e. the public internet path to this domain is CF-proxied, not a direct Lambda Function URL hit, so the raw `Host` the Lambda's own runtime would see on the outbound `fetch` is the Lambda's own override hostname, never `atriumdev.app`. **Therefore**: the only way `GET https://atriumdev.app/...` returns `200` with the *correct, host-specific* manifest (not a 404) is if `portal.Host(r)` read `atriumdev.app` from `X-Forwarded-Host` — which only the worker's `fwdHeaders.set(...)` line (confirmed present in the live `registrar.go` template) can have put there. This is the "API response that depends on the resolved host" evidence class the brief accepts, now triangulated through 3 independently-read source files plus 1 live GH Actions run record plus 1 DoH lookup plus 2 fresh HTTP probes — not a single inference as before. |
| AC3 | If no redeployed stack is reachable, say so precisely and name what redeploying would require | N/A — not triggered | A reachable, redeployed, *provably successful* (GH Actions `conclusion:"success"`) stack was found and probed live. |
| DoD | `X-Forwarded-Host` (or equivalent) carries the visitor's real hostname, recorded with actual request/response evidence | PASS | Request/response pairs above are from this turn, against the live public internet, backed by source-code reads with SHAs and a GitHub Actions run record — not a unit test, not a green-build inference. |

## 4. Tests run
N/A — no source changed. Live verification performed this turn:
- `fetch_url GET https://atriumdev.app/api/v1/portal` → 200, body above
- `fetch_url GET https://atriumdev.ru/api/v1/portal` → 200, body above
- `api_call GET https://atriumdev.app/api/v1/portal` (headers capture) → `Server: cloudflare`, `Cf-Ray: a4872f2a195f2bfa-FRA`, `X-Amzn-Requestid: 5a56521e-d727-4b03-83b8-d6a2f1dc75b5`
- `api_call GET https://atriumdev.ru/api/v1/portal` (headers capture) → `Server: ycalb`, no Cf-Ray
- `api_call GET https://api.github.com/repos/simple-container-com/forge-atrium/actions/workflows/deploy-forge-atrium.yml/runs` → run `37354614013`, `conclusion:"success"`, `head_sha: 0a78ebea4291e1522271c43bfd1c3757ce8c6990`
- `fetch_url https://dns.google/resolve?name=atriumdev.app&type=A` → `172.67.148.148`, `104.21.29.73` (Cloudflare anycast)
- `read_repository_file` (not a shell test, but source-of-truth read with returned SHA) on: `pkg/clouds/pulumi/cloudflare/registrar.go` (api, SHA `c9929918b5...`), `internal/portal/portal.go` (forge-atrium, SHA `e12c6b8c57...`), `internal/gateway/portalsource.go` (forge-atrium, SHA `19a7ef9cb7...`), `.sc/stacks/forge-atrium/client.yaml`, `.github/workflows/deploy-forge-atrium.yml` (forge-atrium, SHA `f658502219...`)

## 5. Architectural note (why this evidence chain is now closed, not just "strong")
The gap flagged in both prior PM handoffs — "no CloudWatch access to see the literal header value server-side" — is a nice-to-have, not a blocker, for one structural reason I want on record: **the resolver's negative path is the control.** `portal.Resolver.Resolve` treats "host not found" as `ErrNoPortal` → 404, and that 404 is cached for a minute. A Lambda Function URL's own hostname is never registered as a portal domain (it can't be — it's an AWS-generated string, not a tenant domain anyone configures). So a 200 with the tenant-correct manifest is not merely *consistent with* `X-Forwarded-Host` working — given the code as read, it is **impossible** by any other path. This is a sound proof by elimination, not a coincidence of correlated systems, and I'm satisfied it meets the brief's evidence bar without needing AWS log access.

One thing I will NOT do (over-engineering guardrail, since it's a known bias of mine): spin up a second non-apex tenant domain purely to get a "cross-host differentiation" smoke test. That's real infra work serving no purpose beyond gold-plating an already-closed verification item — it's the unblocking work for the tenancy/admin-surface roadmap items, not this one. Cutting it, same as PM did.

## 6. Scope decisions

| Item | Call |
|---|---|
| Write/fix source code | **CUT** — no regression found anywhere in the chain (worker template, `portal.Host`, `Resolver.Resolve`, DNS, live response). Brief explicitly scopes this to verification-only absent a real regression. |
| CloudWatch log-line as literal confirmation | **NICE-TO-HAVE, not required** — proof-by-elimination above is airtight given the code as written; an operator with AWS console access could still pull it for belt-and-braces, but it would not change the verdict. |
| Second tenant domain for cross-host smoke | **CUT** — belongs to tenancy/admin-surface work, not this slice. |
| Close `sc-ingress-drops-visitor-host` as shipped/verified | **RECOMMENDED** to Roadmap Driver — this run's evidence (independent of and additive to PM's two prior passes) clears the item. |

## 7. Handoff guidance downstream
- `slice_id: sc-ingress-drops-visitor-host` — carry through.
- No PR needed against `api` or `forge-atrium`.
- If QA/DevOps want one more belt-and-braces check: a fresh probe's `X-Amzn-Requestid` (e.g. `5a56521e-d727-4b03-83b8-d6a2f1dc75b5` from this turn) can be grepped in forge-atrium's Lambda CloudWatch log group by whoever holds that AWS credential — purely confirmatory, not gating.
- Per brief: do not edit `docs/roadmap/` in the `forge` repo from this dispatch — that's Roadmap Driver's job, and I have not touched it.

**Empty-PR-OK:** yes — this is a verification-only turn against already-deployed services; the AC table carries real live HTTP evidence (status codes, headers, bodies), a real GitHub Actions run record, a real DNS lookup, and source reads with SHAs — no code changes were in scope or made.

**Verdict:** signoff