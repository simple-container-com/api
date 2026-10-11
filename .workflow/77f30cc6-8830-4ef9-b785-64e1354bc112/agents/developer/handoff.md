# Developer Handoff — sc-ingress-drops-visitor-host (REDISPATCH, verification-only)

## 0. Headline

**AC3 is PASS by strong inference, not by direct observation.** I got a live 200 from a deployed SC service whose response is *only* reachable if the origin resolved the visitor's real hostname — but I could **not** read a server-side log line that literally prints `X-Forwarded-Host: atriumdev.app`. That distinction matters and I am not going to paper over it: the brief asked for "a log line, a debug endpoint, a header echo", and what I have is a **behavioural discriminator**, which is a different (and in one respect stronger, in another weaker) class of evidence. Details and the exact missing grant in §6.

I also **corrected two upstream errors** that would each have produced a false result if I'd followed the plan as written:

1. **The architect's `/api/v1/portal` → `portalOr404` claim was right, but their route-finding method was broken** — `search_repository_code` returns **zero hits for every query** on `forge-atrium` (even `portal`, in a portal platform). Tree listing returns an empty `internal/`. I stopped trusting that tool and cloned the repo instead. Anyone downstream relying on code-search against this repo is reading a false negative.
2. **The OIDC discovery document is a trap.** `https://atriumdev.app/.well-known/openid-configuration` returns `"issuer":"https://atriumdev.app"`, which looks exactly like a host echo and is the obvious thing to cite. It is **not** host-derived — `cmd/atrium/main.go:571` sets `publicBase := envOr("ATRIUM_ADMIN_PUBLIC_URL", "https://atriumdev.app")` and line 696 passes it as `Issuer`. It would print `atriumdev.app` even if `X-Forwarded-Host` were completely broken. **Do not cite it as evidence.**

## 1. Commit + branch identity

**No code mutations in this turn.** Verification-only dispatch (AC6 is a scope guard forbidding code changes).

- Working tree: `git status --porcelain` → empty (clean)
- HEAD on the run branch: `45d92f8e3f` (unchanged — the architect's handoff commit)
- Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/77f30cc6-8830-4ef9-b785-64e1354bc112`

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files touched. Verification-only turn; AC6 forbids code changes. |

## 3. The evidence, and why the 200 is load-bearing

The chain, every link verified by reading code I cloned rather than inherited:

| Link | Verified how |
|---|---|
| The worker **sets** `X-Forwarded-Host` to the visitor's hostname | `pkg/clouds/pulumi/cloudflare/registrar.go:262,271` on `origin/main`: `const origHost = new URL(origRequest.url).hostname;` → `fwdHeaders.set("X-Forwarded-Host", origHost);` |
| The fix is in `main` | `git merge-base --is-ancestor 5252819589f8c11f4a4a450c1dbcc4602e1913cb origin/main` → **CONFIRMED ancestor**. Commit: `fix(cloudflare): forward the visitor's Host as X-Forwarded-Host (#406)`, 2026-09-23 |
| The deployed stack picked up the fix | `.github/workflows/deploy-forge-atrium.yml:84` pins `simple-container-com/api/.github/actions/deploy-client-stack@main` — **floating `@main`**, so the 2026-10-05 deploy used post-fix SC. `/healthz` → `{"status":"ok","version":"2026.10.05-0a78ebe"}`, matching PM's GHA run `37354614013` / commit `0a78ebea` |
| The origin reads that header first | `internal/portal/portal.go:136-150`: `h := r.Header.Get("X-Forwarded-Host"); if h == "" { h = r.Host }` |
| `/api/v1/portal` resolves **only** on that value, with **no default fallback** | `internal/blocks/blocks.go:494-506` `portalOr404`: `h.res.Resolve(r.Context(), portal.Host(r))`; `ErrNoPortal` → **hard 404**. No apex bypass, no default manifest — I read the whole function to rule that out |
| A registry miss is an exact-match miss, not a catch-all | `portal-resolve-endpoint.md:83-86`: the store does "exact equality against an indexed array"; `conductor.go:218-220` returns `ErrNotFound` when `out.ID == ""` |
| `atriumdev.app` **is** a registered portal domain | `tenant-service-token.md:22`: `POST /api/v1/portals` → 201, **`domains: [atriumdev.app]`**, 2026-10-03 19:36:52Z |

**The live request** (2026-10-11T01:01:11Z):

```
$ curl -sS -D - https://atriumdev.app/api/v1/portal
status=200
server: cloudflare
cf-ray: a489eea91df9f00d-VNO
x-amzn-requestid: a68d14f0-a74b-4237-a3bb-109c4cc84a2a
x-amzn-trace-id: Root=1-6acadfd8-...
{"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},
 "blocks":["sign-in","chat","processes"],
 "assistantName":"Atrium Assistant","chatTransport":"sse"}
```

`server: cloudflare` + `x-amzn-requestid` together prove the request traversed **Cloudflare worker → AWS Lambda Function URL** — the exact topology the fix targets. The body's three blocks match the registered manifest recorded on 2026-10-03.

**Why this is decisive:** `portalOr404` has exactly two outcomes — a manifest, or 404. If `X-Forwarded-Host` were still missing/broken, `portal.Host(r)` would return the Lambda's own `<id>.lambda-url.<region>.on.aws` hostname. That string is not in the registry (exact-match), so `Resolve` would return `ErrNoPortal` and the response would be **404 `{"error":"We could not find that."}`** — which I confirmed is the live 404 shape via `/api/v1/nonexistent`. **I got 200 with the registered tenant manifest. That outcome is unreachable unless the origin saw `atriumdev.app`.**

**Negative/sanity controls I ran:**

| Probe | Result | Reading |
|---|---|---|
| `-H "X-Forwarded-Host: bogus-xfh-probe.invalid"` | **200, byte-identical manifest** | Confirms the worker uses `set()`, not `append()` — it *overwrites* client-supplied XFH (registrar.go:267-268 says so explicitly). The header at the origin is worker-authored and client-spoof-proof. |
| `-H "X-Forwarded-Host: evil.invalid, atriumdev.app"` | 200, identical | Same — overwritten before reaching origin. |
| `-H "Host: bogus.invalid"` | **403** (Cloudflare edge) | Can't reach origin with a forged Host. |
| `GET /` with spoofed XFH vs. baseline | both 200, **both exactly 24361 bytes** | **Non-discriminating — do not use.** `rootHandler` (main.go:1255+) serves the landing on *both* the apex branch *and* the `ErrNoPortal` branch, so root can't distinguish. PM's instinct to avoid the apex was right. |
| `atriumdev.ru/api/v1/portal` | 200, `chatTransport:"websocket"` | **Confounder I had to chase down** — see below. |

**On the `.ru` confounder (this nearly broke my conclusion):** `client.yaml:131` says "atriumdev.ru has no provisioned portals yet", yet `.ru` returns 200. If resolve were a catch-all, my `.app` 200 would prove nothing. Resolution: `.ru` is a **different origin on a different ingress** — `server: ycalb` (Yandex Cloud ALB), no `cf-ray`, no `x-amzn-requestid`, different `chatTransport` (`websocket` vs `sse`) and a different build (`2026.10.05-0d1156d`). It is behind YC API Gateway, **not** the Cloudflare worker (and `yandex/registrar.go:413` forwards XFH by its own separate mechanism). So `.ru` is out of scope for this fix, exactly as PM flagged, and is **not** evidence that resolve is a catch-all. The `client.yaml` comment may simply be stale — I did not chase that further as it's outside this brief.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Fleet stack redeployed since SC 2026.9.16 (2026-09-30), behind the shared CF worker | **PASS** | `atriumdev.app`. Independently re-verified, not just inherited: `/healthz` → `version: 2026.10.05-0a78ebe` (post-release); deploy action pinned `@main` so it built post-fix SC; `server: cloudflare` + `x-amzn-requestid` headers confirm CF-worker→Lambda-URL topology |
| AC2 | Real HTTP request to the stack's public hostname | **PASS** | `curl -sS -D - https://atriumdev.app/api/v1/portal` → 200, `cf-ray: a489eea91df9f00d-VNO`, `x-amzn-requestid: a68d14f0-a74b-4237-a3bb-109c4cc84a2a`, 2026-10-11T01:01:11Z. Full transcript §3 |
| AC3 | Server-side evidence the origin received the visitor's actual Host via XFH, distinct from the Lambda URL's own hostname | **PASS (by behavioural discriminator, NOT by a log line — read the caveat)** | The 200-with-registered-manifest is reachable only if `portal.Host(r) == "atriumdev.app"`; the Lambda-URL hostname would miss the exact-match registry → 404. Chain fully traced in §3. **Caveat:** this is server-side *behaviour* conditioned on the header, not a captured log line printing the header's value. I could not read CloudWatch (§6). A `log.Info("http", ..., "host", portal.Host(r))` line exists at `cmd/atrium/main.go:1452-1459` and *would* print it verbatim — that's the artifact a CloudWatch grant would retrieve |
| AC4 | Find/construct a request that actually exercises the code path (apex short-circuits) | **PASS** | Confirmed PM's apex finding by reading the real file (`apexSet` main.go:151, `isOwnOrigin` main.go:180, `rootHandler` main.go:1255+) — and went further: root `/` is non-discriminating *even on a portal host*, because `ErrNoPortal` also falls through to the landing. `/api/v1/portal` is exempt from the apex set entirely (`portalOr404` never consults `apex`), which is why it works. Verified empirically: both root probes returned identical 24361 bytes |
| AC5 | If no live exercisable path exists, say so precisely | **N/A** | A path existed and was exercised. Noted for the record: a same-origin negative control was **impossible** — `random123.atriumdev.app` is NXDOMAIN (`Status:3`, no wildcard DNS) despite a `*.atriumdev.app` wildcard cert, so TLS fails before HTTP. That's why I used the registered-vs-404-shape discriminator instead |
| AC6 | No code changes — read + live-request only | **PASS** | `git status --porcelain` → empty; HEAD still `45d92f8e3f`. No files touched in `api` or `forge-atrium` (the latter cloned read-only to `/tmp/fa`) |

## 5. Tests run

No code changed, so no build/test gate applies (BUILD-BEFORE-HANDOFF is vacuous here — there is no diff to build). I did **not** run `go build ./...` / `go test ./...`, and deliberately did not run the `registrar.go` unit tests: the brief explicitly states this must not be "a unit test of the template string". There is also no `pkg/clouds/pulumi/cloudflare/*_test.go` asserting the XFH line (I checked — no such test file exists; the `yandex` package has equivalent coverage at `registrar_test.go:167,250`, which is a genuine gap but out of scope here).

Live commands run, verbatim, all against deployed infrastructure — see §3 for outputs:

```
curl -sS -D - https://atriumdev.app/api/v1/portal                              # 200 + manifest
curl -sS -H "X-Forwarded-Host: bogus-xfh-probe.invalid" .../api/v1/portal      # 200 (overwritten)
curl -sS -H "Host: bogus.invalid" .../api/v1/portal                            # 403 at edge
curl -sS .../api/v1/nonexistent                                                # 404 {"error":"We could not find that."}
curl -sS https://atriumdev.app/healthz                                         # 2026.10.05-0a78ebe
curl -sS https://atriumdev.ru/api/v1/portal -D -                               # 200, server: ycalb
openssl s_client -connect atriumdev.app:443 ...                                # SAN: *.atriumdev.app, atriumdev.app
dig via cloudflare-dns.com random123.atriumdev.app                             # Status:3 NXDOMAIN
git merge-base --is-ancestor 5252819589f8... origin/main                       # CONFIRMED ancestor
```

One transient encountered: `GET /app/` timed out at 25s (`curl: (28)`, status=000) — cold-start on the SPA route. Not retried; it was a nice-to-have probe, not load-bearing for any AC, and `/healthz` + `/api/v1/portal` on the same origin answered fine immediately before and after. Flagging rather than hiding it.

## 6. The one thing I could not do — and the exact grant that would close it

**Missing:** CloudWatch Logs read access to the `forge-atrium` staging Lambda's log group (AWS creds for the account hosting the Function URL behind `atriumdev.app`).

**What it would retrieve:** `cmd/atrium/main.go:1452-1459` logs every non-health request as JSON including `"host", portal.Host(r)`. The log line for `x-amzn-requestid: a68d14f0-a74b-4237-a3bb-109c4cc84a2a` (my 01:01:11Z request) will read `"host":"atriumdev.app"`. That is the single artifact that turns AC3 from strong inference into direct observation, and it is already written — no code needed, just read access.

Per the operator-blocker rule I did **not** push this downstream hoping QA resolves it: QA holds `forge_api_key` (the conductor API key), which is **not** an AWS credential and cannot read CloudWatch. Nobody in this chain holds the grant, so handing it on would burn another turn against the same wall.

I also considered and rejected adding a debug/echo endpoint to force a direct observation — that is explicitly scope creep on a "verification, not a code task" brief (AC6), and PM pre-emptively ruled it out.

**My assessment:** AC3 is satisfied for the purpose the brief states — "X-Forwarded-Host arrives at a deployed SC service carrying the hostname the visitor typed, verified against a real request — not a unit test of the template string." The 200 is a real request against deployed infra whose outcome is *causally conditioned* on the header's value, with the 404 counterfactual established from the live service. If the operator wants the literal log line for the record, the CloudWatch grant above is the whole ask. I'd rather state that boundary plainly than call a behavioural discriminator a "log line" and have the next reader discover the difference themselves.

**Recommend** (not doing it from this dispatch — the brief forbids touching `docs/roadmap/` in the `forge` repo, that's the Roadmap Driver's bookkeeping): the stale `client.yaml:131` comment about `.ru` portals, the missing `X-Forwarded-Host` assertion in `pkg/clouds/pulumi/cloudflare/` tests, and the now-obsolete `isOwnOrigin` doc comment in `forge-atrium` (main.go:162-182 still describes the *pre-fix* world — "It copies the client's headers verbatim and adds no X-Forwarded-Host" — which is no longer true and is actively misleading) are each worth a separate roadmap item.

**Empty-PR-OK:** yes — the brief dispatched a live verification of an already-merged-and-released fix; AC6 explicitly forbids source changes, and the AC table carries real live evidence (URLs, status codes, response bodies, `cf-ray`/`x-amzn-requestid` correlation IDs, command output).

**Verdict:** signoff