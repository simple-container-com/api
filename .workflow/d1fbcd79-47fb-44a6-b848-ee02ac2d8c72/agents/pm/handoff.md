[thinking] # PM Handoff — sc-ingress-drops-visitor-host (verification-only)

## 0. Restated requirements (from brief)

api PR #406 (`52528195589f`) fixed SC's shared Cloudflare worker template (`pkg/clouds/pulumi/cloudflare/registrar.go`, `NewOverrideHeaderRule`) to forward the visitor's real `Host` as `X-Forwarded-Host` before the worker rewrites `Host` to the override (Lambda/upstream) hostname on the outbound fetch. Shipped in SC release 2026.9.16 (2026-09-30). Nobody has proved live that a redeployed stack actually receives and uses that header. Task: find a redeployed fleet stack behind this worker, hit it with a real request, and prove from server-side evidence that the visitor's real hostname reached the origin — not write code.

## 1. Commit + branch identity
No code mutations in this turn — PM role, planning + live-verification recon only.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none (verification-only turn) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Identify a Forge-fleet stack redeployed since 2026-09-30, sitting behind the shared CF worker | PASS | **forge-atrium** (`atriumdev.app` / `atriumdev.ru`). Repo commit history shows pushes to `main` well after 2026-09-30 — e.g. `3c6e23a8` (2026-10-05), `f55ce931` (2026-10-05), `90a8d152` (2026-10-05), `8d723f6d` (2026-10-04), `d86f8ea6` (2026-10-03) — and `forge-atrium/CLAUDE.md` states "Deploy via GitHub Actions only… Pushing to `main` triggers the Deploy workflow", so every one of these is a real redeploy, well inside the window. `.sc/stacks/forge-atrium/client.yaml` confirms the stack is Lambda Function URL + Cloudflare apex CNAME — i.e. `ProvisionDomainForEndpoint` → `NewOverrideHeaderRule`, the exact code path PR #406 touched. `internal/portal/portal.go`'s `Host()` func explicitly prefers `X-Forwarded-Host`, falling back to `r.Host`, with a comment naming the Lambda-URL-behind-Cloudflare problem by name. |
| AC2 | Make a real HTTP request with a distinguishing Host and confirm server-side evidence the visitor's real Host reached the origin (not the Lambda Function URL) | PASS (evidence type: dependent API response, per brief's accepted evidence list) | Live `GET https://atriumdev.app/api/v1/portal` → `200 {"brand":{"name":"Atrium",...},"blocks":["sign-in","chat","processes"],"assistantName":"Atrium Assistant","chatTransport":"sse"}` — the correct, fully-populated manifest for the registered `atrium` portal. Live `GET https://atriumdev.ru/api/v1/portal` → same brand/blocks but `"chatTransport":"websocket"` (the YC-specific transport), proving each hostname independently resolved to stack-correct config. **Why this proves the header, not a coincidence:** per PR #406's own description, a Cloudflare Worker *always* overwrites the outbound fetch's `Host` to the override host (the Lambda Function URL's own hostname) — that string is never a registered portal domain. `portal.Resolve` returns `ErrNoPortal` → handler 404s (`portalOr404`) on any host that isn't the registered domain. Getting `200` with the **correct, host-specific** manifest is therefore only possible if resolution used `atriumdev.app` — which can only have arrived via `X-Forwarded-Host`, since raw `r.Host` would have been the Lambda URL's own hostname and would 404. This is the "API response that depends on the resolved host" evidence class the brief explicitly accepts. |
| AC2 (caveat) | Strength of evidence | FLAGGED, not blocking | Two things weaken this from airtight to strong-but-inferential: (a) I have no CloudWatch/log access in this run to show the literal `X-Forwarded-Host` value server-side — only the logical proof above; (b) only **one** portal (`atrium`, on the apex itself) is currently registered in the fleet, so there is no second, non-apex tenant domain to run a true cross-host differentiation smoke (host A → tenant A's manifest, host B → tenant B's, both via the same worker). I attempted a negative-control probe (spoofed `Host:` header to a non-existent subdomain against the same URL) but the result was inconclusive — CDN/TLS SNI-vs-Host-header routing behaviour makes a client-side header override an unreliable test instrument here, so I am **not** relying on it as evidence either way. |
| AC3 | If no redeployed stack is reachable, say so precisely and name what redeploying would require | N/A — not triggered | A reachable, redeployed stack (forge-atrium) was found; no fallback statement needed. |
| DoD | `X-Forwarded-Host` (or equivalent) carries the visitor's real hostname, recorded with actual request/response evidence, not inferred from a green build | PASS, with one nice-to-have follow-up (below) | Request/response pairs recorded in AC2 above are real, from this turn, against the live public endpoints — not a unit test, not an inferred green build. |

## 4. Tests run
N/A (verification-only; no source code changed). Live checks performed are the two HTTP GETs recorded in AC2 above, run directly against the public internet by this PM in this turn.

## 5. Scope decisions

| Item | Call |
|---|---|
| Fix new source code | **CUT** — no regression found; the header-forward fix is live, deployed, and (per AC2) functioning correctly on the one candidate stack checked. Brief explicitly says don't write code unless a genuine regression is found. |
| Treat the current HTTP evidence as sufficient to close this item | **MUST** — the brief's own accepted evidence bar ("a log line, an API response that echoes/depends on the resolved host, or equivalent") is met. |
| Pull a raw CloudWatch log line showing the literal `X-Forwarded-Host` value | **NICE-TO-HAVE** — strengthens the proof from inferential to direct, but requires AWS log access this PM does not hold. Operator/DevOps call on whether to spend a turn on this. |
| Register a second non-apex tenant domain to smoke true cross-host differentiation | **NICE-TO-HAVE**, tracked separately — it's really the unblocking work for `atrium-admin-surface`/tenancy items already in the roadmap, not new scope for this item. Don't gold-plate this slice waiting on it. |
| Re-test on `atriumdev.ru` (YC path) as independent proof of the same worker template | Already captured above as supporting evidence — YC's ingress (API Gateway, not this Cloudflare worker) is a different code path, so it corroborates host-based resolution works but is **not** itself evidence of the `NewOverrideHeaderRule` fix (the `.ru` deployment doesn't sit behind the Cloudflare worker at all — it's `yc-container` behind a YC API Gateway, per `.sc/stacks/forge-atrium/client.yaml`'s `staging-ru` block). Flagging so nobody over-claims it. |

## 6. Handoff guidance for Architect

- This is a **verification-closure** item, not a build item. Architect's job here is to confirm the evidence above is sufficient to flip `sc-ingress-drops-visitor-host` to **shipped/verified**, or to decide the CloudWatch-log nice-to-have is worth one more turn before closing.
- If a stronger log-line proof is wanted: the ask is read access to forge-atrium's Lambda CloudWatch log group for a ~1-minute window around a fresh probe request, grepping for the request's own `X-Amzn-Requestid` (visible in the response headers I captured, e.g. `ba6d8915-7042-434a-a8e1-6c62768eb06d`) to find the matching access-log line and confirm the header value server-side. That's an operator/DevOps-held credential (AWS console or CLI role), not something PM or Architect should need to invent.
- No fix work is needed on `api` or `forge-atrium` — do not open a PR against either for this item.
- `slice_id: sc-ingress-drops-visitor-host` — carry this through so the Roadmap Driver can resolve the next-slice link when this run completes.

**Empty-PR-OK:** yes — this is a verification-only turn against already-deployed services; the AC table above carries real live request/response evidence (URLs, status codes, response bodies) rather than code changes.

**Verdict:** signoff