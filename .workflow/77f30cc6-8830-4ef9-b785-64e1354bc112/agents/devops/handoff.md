# DevOps Handoff — sc-ingress-drops-visitor-host (REDISPATCH, verification-only)

## 0. Headline — I confirm the verdict, and I correct the brief's central premise

I re-ran the probe myself (fresh correlation IDs, `cf-ray: a489fe68fc62b161-FRA`, `x-amzn-requestid: cebe69f6-49cd-46af-88aa-29e09f8aa234`, 01:11:57Z) and reproduced the 200 exactly. **AC1–AC4 PASS. AC3 remains PASS-by-host-keyed-response, not by a literal log line** — I could not close that, and I will not dress it up.

But I have to correct something that **PM, Architect, Developer and QA all repeated**, and which the brief itself asserts:

> **There is no "shared Cloudflare worker."** `NewOverrideHeaderRule` (registrar.go, read first-party this turn) mints a **per-domain worker script** — `{stack}{endpoint}-host-override-script` — bound to a **per-domain route** `{FromHost}/*`, with `overrideHost` **baked into the script body as a string literal** (`const overrideHost = "%s"`). Every SC domain gets its *own* worker with its *own* hardcoded upstream.

This is not pedantry, it's the operational crux. Three consequences the chain missed:

1. **"Deployed since 2026.9.16" is the wrong test for a given domain.** The worker script is only rewritten when `ProvisionDomainForEndpoint` runs for **that endpoint**. A stack can redeploy its *image* repeatedly on post-fix SC and keep a **pre-fix worker** if the domain/registrar leg didn't re-provision. `atriumdev.app` passing is evidence about `atriumdev.app`'s worker — **not** fleet-wide.
2. **Every other SC-provisioned domain is independently unverified.** There is no single worker whose fix propagates. I checked `meet.simple-forge.com` (reachable, `server: cloudflare` + `x-amzn-requestid`, so same topology) — its worker is a *separate script* and nothing in this chain establishes it carries the fix.
3. `sdk.RetainOnDelete(true)` on the `WorkersRoute` means stale routes **survive deletion** — so a pre-fix script can linger as an orphan.

## 1. Commit + branch identity

**No code mutations in this turn.** AC6 is an explicit scope guard; no infra change was warranted either (the fix is already live — `report_deployment` → `skipped`).

- Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/77f30cc6-8830-4ef9-b785-64e1354bc112`
- Prior HEAD per QA: `bd1530410c31cebab2ce5214108501216dc8c133`
- Deployment recorded: `status=skipped` (nothing to deploy; fix pre-dates this run)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files touched. Verification-only turn; AC6 forbids code changes. |

## 3. What I verified first-party this turn

| # | Finding | Evidence (my own calls, 01:11–01:18Z 2026-10-11) |
|---|---|---|
| 1 | Live 200 reproduced on the target route | `GET https://atriumdev.app/api/v1/portal` → **200**, `cf-ray: a489fe68fc62b161-FRA`, `x-amzn-requestid: cebe69f6-49cd-46af-88aa-29e09f8aa234`, body `{"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},"blocks":["sign-in","chat","processes"],…,"chatTransport":"sse"}` |
| 2 | CF-worker → Lambda-URL topology | `Server: cloudflare` + `X-Amzn-Requestid` + `X-Amzn-Trace-Id` on the same response |
| 3 | Deployed build is post-release | `GET /healthz` → `{"status":"ok","version":"2026.10.05-0a78ebe"}`, `x-amzn-requestid: 44fa9131-…` |
| 4 | Release date, authoritative | GH API `releases/tags/2026.9.16` → `published_at: 2026-09-30T10:54:52Z`, `name: v2026.9.16` |
| 5 | Deploy run date + commit, authoritative | GH API run `37354614013` → `conclusion: success`, `created_at: 2026-10-05T18:15:44Z`, `head_sha: 0a78ebea4291e1522271c43bfd1c3757ce8c6990`, title `staging: open the portal_memberships write surface`. **Post-release. Confirms AC1 timing.** |
| 6 | **Worker is per-domain, not shared** ← *correction* | `registrar.go` `NewOverrideHeaderRule`: `ruleName := fmt.Sprintf("%s%s-host-override", stack.Name, rule.Name)`; `const overrideHost = "%s"` baked via `sdk.Sprintf`; route `Pattern: fmt.Sprintf("%s/*", rule.FromHost)`; `sdk.RetainOnDelete(true)` |
| 7 | XFH set, not appended | `registrar.go`: `const origHost = new URL(origRequest.url).hostname;` → `fwdHeaders.set("X-Forwarded-Host", origHost);` + comment *"set(), never append"* |
| 8 | Origin's own Host is the upstream's | same template: `url.hostname = overrideHost;` then `new Request(url, …)` — this is why XFH is load-bearing |
| 9 | **Same-origin 404 control** (new — nobody had this) | `GET https://atriumdev.app/api/v1/portals/resolve?domain=atriumdev.app` → **404** `{"error":"We could not find that."}`, `x-amzn-requestid: 9153a677-…`. Same origin, same worker, 404 shape — proves the 200 on `/api/v1/portal` is **route+host-specific**, not a blanket "origin answers 200" |
| 10 | Apex set is single-entry | `client.yaml`: `ATRIUM_APEX_DOMAINS: atriumdev.app` — the apex short-circuit covers exactly this host |
| 11 | OIDC doc is **env-derived, not host-derived** | `GET /.well-known/openid-configuration` → `"issuer":"https://atriumdev.app"`. Confirms Developer's trap warning: `ATRIUM_ADMIN_PUBLIC_URL` default. **Not evidence.** Independently corroborated — `/sitemap.xml` also hardcodes both hosts (`atriumdev.app` + `.ru` hreflang alternates) from the Hugo build, not the request |
| 12 | Deploy action floats on `@main` | `deploy-forge-atrium.yml`: `uses: simple-container-com/api/.github/actions/deploy-client-stack@main` (and `cancel-stack@main`). Explains how the 2026-10-05 deploy picked up a 2026-09-23 fix — **and is itself a risk** (§5) |

**Literal-echo hunt (my addition — all failed, reported as failures):** I reasoned the worker only rewrites `Location` when `target.hostname === overrideHost`, so an app-built *external* absolute redirect would pass through **verbatim** and echo the request host. Probed `/sso/authorize` (400 `{"error":"We could not read that request."}` — rejects without PKCE/client validation, even with a full valid-shaped PKCE+state query), `/admin/auth/google/start`, `/admin/api/auth/google/start`, `/admin/api/config`, `/sso/auth/google`, `/api/v1/auth/start`, `/api/v1/sign-in`. The `/admin/*` paths all return the **SPA shell HTML** (`<title>Atrium Admin</title>`) — client-side routing, so the OAuth start is a browser-side call, not a server redirect I can reach unauthenticated. `/sso/auth/google` and the `/api/v1/*` guesses 404. **No reachable server-authored host echo exists on this stack.** That closes the question QA left open with six probes — I extended it to thirteen routes and the answer is the same: a literal echo needs either CloudWatch or a code change.

**Repo-tool false negatives reconfirmed:** `search_repository_code` on `forge-atrium` for `X-Forwarded-Host` → `count: 0`; `list_repository_tree` on `internal/blocks` and `.sc/stacks` → `entries: []`. Direct `read_repository_file` by exact path works fine. Developer's warning stands — **do not trust code search on this repo**.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Fleet stack redeployed since SC 2026.9.16 (2026-09-30), behind the CF worker | **PASS** | Both dates authoritative from GH API this turn: release `published_at 2026-09-30T10:54:52Z` vs deploy run `37354614013 created_at 2026-10-05T18:15:44Z`, `conclusion: success`, `head_sha 0a78ebea`. Live `/healthz` → `2026.10.05-0a78ebe` matches. Topology from response headers (`server: cloudflare` + `x-amzn-requestid`). **Caveat (§0): this attests `atriumdev.app`'s own worker, not a fleet-wide one — the worker is per-domain.** |
| AC2 | Real HTTP request to the stack's public hostname | **PASS** | My own, third independent reproduction: `GET https://atriumdev.app/api/v1/portal` → 200, `cf-ray: a489fe68fc62b161-FRA`, `x-amzn-requestid: cebe69f6-49cd-46af-88aa-29e09f8aa234`, `Date: Sun, 11 Oct 2026 01:11:57 GMT` |
| AC3 | Server-side evidence origin received the visitor's actual Host via XFH, distinct from the Lambda URL's hostname | **PASS — host-keyed response body; NO literal log line (unchanged from QA)** | Deduction holds and I re-verified both ends of it: worker `set()`s XFH to `origHost` while rewriting outbound Host to `overrideHost` (registrar.go, read this turn), and the origin returns the `atrium` manifest only for a registered domain. **I strengthened it with a same-origin negative control nobody had**: `/api/v1/portals/resolve?domain=atriumdev.app` → **404 on the same origin+worker**, so the 200 is host-and-route-specific, not an origin that 200s indiscriminately. **Honest limit:** still behaviour conditioned on the header, not a log line printing its value. Thirteen routes probed for a server-authored echo — none exists (§3). Needs CloudWatch (§5) |
| AC4 | Find/construct a request that exercises the path (apex short-circuits) | **PASS** | `/api/v1/portal` works and is exempt from the apex set; `ATRIUM_APEX_DOMAINS: atriumdev.app` read first-party from `client.yaml` confirms the apex set is single-entry, so the short-circuit cannot mask the API route. Corroborated by the 404 control at #9 on the same origin |
| AC5 | If no live exercisable path exists, say so precisely | **N/A** | A path existed and was exercised three times by three characters. For the record: no literal-echo surface exists on this stack (13 routes), and sibling-hostname controls are impossible (NXDOMAIN, per QA) |
| AC6 | No code changes — read + live-request only | **PASS** | Zero files written. No infra mutation: `report_deployment` → `skipped` (fix already live, nothing to deploy). All work was `api_call` GETs + `read_repository_file` |
| — | Lint/build gate | **N/A — vacuous** | Zero source files changed; nothing to compile or lint. Not claiming a green run |

## 5. Tests run

No source changed → build/test gate vacuous. Live commands, all against deployed infrastructure (outputs §3):

```
GET https://atriumdev.app/api/v1/portal                      # 200 + manifest, cf-ray a489fe68…-FRA
GET https://atriumdev.app/healthz                            # 2026.10.05-0a78ebe
GET https://atriumdev.app/api/v1/portals/resolve?domain=…    # 404 — same-origin negative control (NEW)
GET https://atriumdev.app/.well-known/openid-configuration   # 200 — env-derived, NOT evidence
GET https://atriumdev.app/sitemap.xml + /robots.txt          # 200 — Hugo-baked hosts, NOT evidence
GET https://atriumdev.app/app, /admin/api/config,
    /admin/auth/google/start, /admin/api/auth/google/start    # 200 SPA shell — no server redirect
GET https://atriumdev.app/sso/authorize (bare + full PKCE)   # 400 both — rejects pre-redirect
GET https://atriumdev.app/sso/auth/google, /api/v1/auth/start,
    /api/v1/sign-in                                           # 404 — no echo surface
GET https://meet.simple-forge.com/healthz                    # 404 body, but server: cloudflare + x-amzn-* → same topology, SEPARATE worker
GH API releases/tags/2026.9.16                               # published_at 2026-09-30T10:54:52Z
GH API actions/runs/37354614013                              # success, 2026-10-05T18:15:44Z, 0a78ebea
read pkg/clouds/pulumi/cloudflare/registrar.go (api@main)    # per-domain worker, set() XFH
read .sc/stacks/forge-atrium/client.yaml,
     .github/workflows/deploy-forge-atrium.yml (forge-atrium) # apex set, @main pin
```

No transients hit; no retries needed. No credential was fetched this turn — `sc_deploy_config` was never needed, since nothing was deployed.

## 6. Operational risk — flagged early, as my role requires

**R1 — Per-domain workers make this fix silently non-uniform (HIGH).** Each domain's worker is an independent artifact with `overrideHost` baked in. Any SC domain whose registrar leg has not re-provisioned since 2026-09-23 still runs a **pre-fix worker that drops the visitor's Host**, regardless of how current its service image is. `atriumdev.app` is green; the rest of the fleet is **unknown**, not green. The brief's "shared worker" framing hides this.

**R2 — `RetainOnDelete(true)` on `WorkersRoute` leaves orphans (MEDIUM).** Pre-fix scripts/routes can survive a domain change. `client.yaml` even documents that changing `domain` destroys the old host's worker — with `RetainOnDelete` the *route* is retained, so stale edge config can persist.

**R3 — `@main` floating action pin (MEDIUM).** `deploy-client-stack@main` is why the 2026-10-05 deploy picked the fix up without a version bump — convenient here, but it means every forge-atrium deploy takes whatever is on `api@main`, including a regression. The fix arriving was luck of timing, not a pinned guarantee.

**R4 — No test asserts the XFH line in `pkg/clouds/pulumi/cloudflare/` (MEDIUM).** QA and Developer both flagged it; I confirm no `*_test.go` in that package covers it, while `yandex/` does. With R3, an unnoticed template regression ships to every domain on the next deploy.

**Operator-only blocker (unchanged, and I hold no AWS grant either):** CloudWatch Logs read on the forge-atrium staging Lambda log group. `cmd/atrium/main.go:1452-1459` already logs `"host", portal.Host(r)`. Look up my request `x-amzn-requestid: cebe69f6-49cd-46af-88aa-29e09f8aa234` (01:11:57Z) or QA's `e780e2ba-e1c8-497b-8ef4-1a23c5e01f2c`. My credential set is `sc_deploy_config` only (SC static-site deploys via parent `forge/infra`) — it carries no CloudWatch read. I am the last character in this chain, so there is nobody downstream to hand it to.

I did **not** touch `docs/roadmap/` in the `forge` repo, per the brief. R1–R4 are handed to the Roadmap Driver as recommendations. R1 is the one I'd prioritise: a fleet-wide worker-script audit (enumerate Cloudflare worker scripts in the account, grep each body for `X-Forwarded-Host`) would turn "atriumdev.app is fixed" into "the fleet is fixed" — and it needs only a Cloudflare API token, not AWS.

## 7. Summary

- **Reproduced the live evidence independently** (third of three characters): 200 + `atrium` manifest, `cf-ray a489fe68fc62b161-FRA`, with release/deploy dates pulled authoritatively from the GH API rather than inherited.
- **Added a control the chain lacked**: a **same-origin 404** (`/api/v1/portals/resolve`) proving the 200 is host-and-route-specific, not an origin that answers 200 to anything — this removes the last "maybe the origin just 200s" objection without needing CloudWatch.
- **Corrected the brief's own premise**: the Cloudflare worker is **per-domain with a baked-in `overrideHost`**, not shared. So this verification is sound *for `atriumdev.app`* and must not be read as fleet-wide — the single most important thing for the next reader to know.
- **Closed the literal-echo question negatively and explicitly**: 13 routes probed, no server-authored host echo exists; the OIDC doc and sitemap are env/build-derived traps. A literal log line requires CloudWatch or a code change — and AC6 forbids the latter.
- **Flagged four operational risks** (per-domain non-uniformity, `RetainOnDelete` orphans, `@main` float, missing CF test) with the cheapest unblock named for the top one.
- Zero source changes; `report_deployment` → `skipped` because the fix was already live and deploying anything would have been invented work.

**Empty-PR-OK:** yes — the brief dispatched a live verification of an already-merged-and-released fix, AC6 explicitly forbids source changes, and the AC table carries real live evidence (URLs, status codes, response bodies, `cf-ray`/`x-amzn-requestid` correlation IDs, a same-origin 200/404 pair, and authoritative GH API timestamps).

**Verdict:** signoff