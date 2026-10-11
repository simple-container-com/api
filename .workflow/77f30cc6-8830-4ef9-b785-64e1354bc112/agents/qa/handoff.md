All links verified first-party. Here's my handoff.

---

# QA Handoff — sc-ingress-drops-visitor-host (REDISPATCH, verification-only)

## 0. Headline — verdict and the one correction that matters

**I independently re-ran the verification rather than accepting the developer's transcript, and I upgrade AC3 from their "strong inference" to a deductive closure — after closing the link they left asserted-but-unverified.**

The developer's chain was structurally correct but had a load-bearing gap: their proof that *"a registry miss returns 404"* and that *"`atriumdev.app` is registered"* was cited from **prior-run markdown docs** (`portal-resolve-endpoint.md:83-86`, `tenant-service-token.md:22`) — not from live state. That is exactly the class of citation that looks like evidence and isn't: if `Resolve` were a catch-all, their 200 would prove nothing, and a doc from a dead run cannot settle it. I established both links live against the conductor registry using the `forge_api_key` grant (which the developer correctly noted they held no AWS creds for, but did not try for the *registry* read).

**One substantive correction to the developer's findings:** they reported `.ru` as having "no provisioned portals" and the `client.yaml:131` comment as *"may simply be stale"*. The live registry disproves that — portal `atrium` lists `domains: ["atriumdev.app","atriumdev.ru"]`. Both are registered. Their `.ru` 200 is explained by **registration**, not by a stale comment, and their inability to explain it was what "nearly broke" their conclusion. The confounder they chased was never a confounder.

**Residual limitation, stated plainly and not papered over:** no literal server-side log line was observed. No CloudWatch grant exists in this chain, and I confirmed there is no debug/echo surface (six candidate routes probed, all 404). The evidence is a host-keyed response body, which the brief's DoD admits ("a log line, a debug endpoint, a header echo, **etc.** — not just 'it didn't error'"). This is materially more than "it didn't error": the response *content* is keyed to the hostname.

**Gate A/B note:** this brief dispatches a live-fleet verification against an **already-deployed** stack. It does not mention Gate A (isolated-stack deploy) or Gate B, and no isolated stack is involved — so `deploy-isolated-stack.yml` / `destroy-isolated-stack.yml` are not applicable and were correctly not dispatched. I am not citing a waiver I don't have; I am citing that the brief's scope is a fleet-stack probe, not an isolated-stack slice.

## 1. Commit + branch identity

**No code mutations in this turn** — AC6 is an explicit scope guard forbidding them.

- Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/77f30cc6-8830-4ef9-b785-64e1354bc112`
- HEAD: `bd1530410c31cebab2ce5214108501216dc8c133` — `forge: character developer (David Black) phase run idempotency: developer:run`
- Working tree: clean apart from untracked `.forge-prompt.md` (harness-injected prompt file, not mine to commit)
- Diff vs `origin/main`: **13 files, +527/-0 — zero Go files** (`git diff origin/main..HEAD --name-only | grep -c '\.go$'` → `0`). All paths are `.workflow/<runId>/` engine metadata plus a `.gitignore` edit that was auto-added by forge-agent-worker on an earlier phase (not authored by me).

Per the substance gate: a diff of only `.workflow/` metadata would normally force `failure`. It does **not** here because the brief dispatched a verification of an already-merged-and-released fix and forbade source changes — the sanctioned `Empty-PR-OK` path, declared below with live evidence attached.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files touched. Verification-only turn; AC6 forbids code changes. |

## 3. The evidence chain — every link re-verified by me this turn

| # | Link | How I verified it (first-party, this turn) |
|---|---|---|
| 1 | Fix is in `main` | `git merge-base --is-ancestor 5252819589f8c11f4a4a450c1dbcc4602e1913cb origin/main` → **CONFIRMED ancestor**. Commit: `fix(cloudflare): forward the visitor's Host as X-Forwarded-Host (#406)`, Ilya, Wed Sep 23 22:05:52 2026 |
| 2 | Worker **sets** XFH to visitor host | `pkg/clouds/pulumi/cloudflare/registrar.go` on `origin/main`: L262 `const origHost = new URL(origRequest.url).hostname;` → L271 `fwdHeaders.set("X-Forwarded-Host", origHost);` |
| 3 | **Origin's own `Host` is NOT the visitor's** (the premise that makes the whole proof work) | Same template, L261: `url.hostname = overrideHost;` then the outbound `new Request(url, …)`. The worker rewrites the outbound Host to the upstream's hostname. Comment L267-268 states it: *"Workers overwrite Host on the outbound fetch with the override hostname, so an upstream that serves more than one domain cannot tell which one the visitor asked for."* |
| 4 | Origin reads XFH first | `forge-atrium` `internal/portal/portal.go:136-140`: `h := r.Header.Get("X-Forwarded-Host"); if h == "" { h = r.Host }` (cloned fresh; doc comment L133-135 names `CF_LAMBDA_URL_DROPS_HOST`) |
| 5 | `/api/v1/portal` resolves **only** on that value — no fallback | `internal/blocks/blocks.go:494-506` `portalOr404`: `h.res.Resolve(r.Context(), portal.Host(r))`; `ErrNoPortal` → hard 404; any other error → 503. Read the whole function + `Portal` handler (L140-145). No default manifest, no apex consult. |
| 6 | **Registry resolve is exact-match, not catch-all** ← *the link the developer asserted from a dead run's doc* | Live, authenticated: `GET app.simple-forge.com/api/v1/portals/resolve?domain=…` → **404 `{"error":"Not Found","message":"portal not found"}`** for all three of `qa-xfh-control-77f30cc6.invalid`, `abcdef123.lambda-url.eu-central-1.on.aws`, `random-unregistered.example.com` |
| 7 | `atriumdev.app` **is** registered, and to which manifest | Live: resolve → **200** `{"id":"atrium","owningOrg":"simple-container-com","domains":["atriumdev.app","atriumdev.ru"],"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},…}` |
| 8 | Deployed build is post-release | `GET https://atriumdev.app/healthz` → `{"status":"ok","version":"2026.10.05-0a78ebe"}` — 2026-10-05, after SC 2026.9.16 (2026-09-30) |

**My own live request** (fresh — distinct correlation IDs from the developer's):

```
$ curl -sS -D - https://atriumdev.app/api/v1/portal
status=200  time=0.467s
date: Sun, 11 Oct 2026 01:05:51 GMT
content-type: application/json; charset=utf-8
server: cloudflare
cf-ray: a489f57a29af7326-WAW
x-amzn-requestid: e780e2ba-e1c8-497b-8ef4-1a23c5e01f2c
{"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},"blocks":["sign-in","chat","processes"],"assistantName":"Atrium Assistant","chatTransport":"sse"}
```

`server: cloudflare` + `x-amzn-requestid` together confirm the **Cloudflare worker → AWS Lambda Function URL** topology the fix targets.

**The deduction (why this is closure, not just inference):**

1. The returned brand (`name: Atrium`, `accent: #6d5cff`) matches **exactly** the live registry record for portal `atrium`, whose domains are precisely `[atriumdev.app, atriumdev.ru]`. So `portal.Host(r)` resolved to one of those two strings — nothing else maps to this manifest (link 6: exact-match).
2. It was not `atriumdev.ru`: that is a **different origin on a different ingress** — `server: ycalb` (Yandex ALB), no `cf-ray`, no `x-amzn-requestid`, `chatTransport: websocket` vs `sse`. My request carried `cf-ray` + `x-amzn-requestid`.
3. ∴ `portal.Host(r) == "atriumdev.app"`.
4. `portal.Host(r)` returns **either** `X-Forwarded-Host` **or**, only when that is empty, `r.Host` (link 4). At this origin `r.Host` is the worker's `overrideHost` — the Lambda Function URL hostname, not `atriumdev.app` (link 3, confirmed by the AWS response headers).
5. ∴ the value came from **`X-Forwarded-Host`, carrying the hostname the visitor typed**. ∎

**Controls I ran:**

| Probe | Result | Reading |
|---|---|---|
| `-H "X-Forwarded-Host: qa-xfh-control-77f30cc6.invalid"` | 200, **byte-identical** to baseline (155 B, `diff -q` → identical) | Worker uses `set()` not `append()` — client-supplied XFH is overwritten. Origin-side value is worker-authored and **spoof-proof**. |
| Debug/echo surface hunt: `/api/v1/debug`, `/debug`, `/debug/headers`, `/api/v1/whoami`, `/api/v1/echo`, `/headers` | all **404** | No header-echo endpoint exists → a literal echo artifact is genuinely unavailable without code changes (which AC6 forbids). |
| Same-origin negative control: DNS for `www`/`app`/`admin`/`portal`.atriumdev.app | all **NXDOMAIN** (`"Status":3`) | No wildcard DNS → an unregistered-host-same-origin control is impossible. Corroborates the developer's finding; this is why the registry-side 404 (link 6) is the right substitute. |
| `conductor.simple-forge.com` | `SSL_ERROR_SYSCALL`, DNS `"Status":3` | **My own wrong hostname, not a transient** — I checked DNS before retrying rather than burning the retry budget. Correct host is `app.simple-forge.com` (DNS `Status:0`, `104.21.91.30`). Logging it so nobody downstream reads it as an outage. |

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Fleet stack redeployed since SC 2026.9.16 (2026-09-30), behind the shared CF worker | **PASS** | `atriumdev.app`. `/healthz` → `version: 2026.10.05-0a78ebe` (post-release, verified by me this turn). `server: cloudflare` + `x-amzn-requestid` confirm CF-worker→Lambda-URL topology. Fix confirmed in `main` via `git merge-base --is-ancestor` → CONFIRMED |
| AC2 | Real HTTP request to the stack's public hostname | **PASS** | My own request, not inherited: `curl -sS -D - https://atriumdev.app/api/v1/portal` → 200, `cf-ray: a489f57a29af7326-WAW`, `x-amzn-requestid: e780e2ba-e1c8-497b-8ef4-1a23c5e01f2c`, `date: Sun, 11 Oct 2026 01:05:51 GMT`. Full transcript §3 |
| AC3 | Server-side evidence the origin received the visitor's actual Host via XFH, distinct from the Lambda URL's own hostname | **PASS** (host-keyed response body; **no literal log line** — see caveat) | Five-step deduction in §3 closes it: response body matches the live registry record for a manifest keyed *only* to `atriumdev.app`/`.ru`; `.ru` excluded by ingress headers; `r.Host` at the origin is the worker's `overrideHost` per template L261 — so the resolved value can only have arrived via XFH. The counterfactual is **live, not asserted**: registry resolve returns 404 for a lambda-url-shaped hostname. **Caveat:** server-side *behaviour conditioned on the header*, not a captured log line printing its value. `cmd/atrium/main.go:1452-1459` logs `"host", portal.Host(r)` and would print it verbatim; retrieving it needs a CloudWatch grant nobody in this chain holds (§6) |
| AC4 | Find/construct a request that exercises the code path (apex short-circuits) | **PASS** | `/api/v1/portal` → `Portal` (blocks.go:140-145) → `portalOr404` (L494-506) → `portal.Host(r)`, unconditionally and with no apex consult — read the full functions myself. Apex avoidance corroborated: the developer measured both root probes at identical 24361 B, and I confirmed `portalOr404` has no fallback branch that could mask a miss |
| AC5 | If no live exercisable path exists, say so precisely | **N/A** | A path existed and was exercised. Recorded for the next reader: same-origin negative control is impossible (all sibling hostnames NXDOMAIN), and no debug/echo route exists (6 probed, all 404) — hence the registry-side 404 as the counterfactual |
| AC6 | No code changes — read + live-request only | **PASS** | `git diff origin/main..HEAD --name-only` → 0 `.go` files; every path is `.workflow/` metadata + a worker-authored `.gitignore`. Working tree clean except harness-injected `.forge-prompt.md`. `forge-atrium` cloned read-only to `/tmp/fa`, then deleted |
| — | **Static-lint gate** (standing rule) | **N/A — vacuous** | `.golangci.yml` **does** exist at repo root (no `make lint` target), so the gate is in scope by configuration. But **zero source files changed**, so there is nothing to lint, and no Go toolchain is installed in this worker (`which go` → not found). I did **not** run it and am not claiming a green lint — I'm reporting it as inapplicable to a zero-diff turn rather than silently omitting the row |

## 5. Tests run

No source changed, so the build/test gate is vacuous — there is no diff to compile. I did not run `go build ./...` / `go test ./...` (no Go toolchain present; `which go` → not found), and deliberately did **not** run the `registrar.go` template unit tests: the brief explicitly excludes "a unit test of the template string" as evidence.

Live commands executed this turn, all against deployed infrastructure (outputs in §3):

```
git merge-base --is-ancestor 5252819589f8… origin/main              # CONFIRMED ancestor
git grep -n "X-Forwarded-Host" origin/main -- pkg/clouds/.../cloudflare/   # L267,268,271
curl -sS -D - https://atriumdev.app/api/v1/portal                   # 200 + manifest, cf-ray a489f57a…-WAW
curl -sS -H "X-Forwarded-Host: qa-xfh-control-…invalid" …/portal    # 200, byte-identical (155 B)
curl -sS https://atriumdev.app/healthz                              # 2026.10.05-0a78ebe
curl -sS -D - https://atriumdev.ru/api/v1/portal                    # 200, server: ycalb (different ingress)
curl …/api/v1/portals/resolve?domain=atriumdev.app                  # 200, domains:[atriumdev.app,atriumdev.ru]
curl …/api/v1/portals/resolve?domain=abcdef123.lambda-url.…on.aws   # 404 portal not found
curl …/api/v1/portals/resolve?domain=qa-xfh-control-….invalid       # 404 portal not found
curl …/api/v1/portals/resolve?domain=random-unregistered.example.com # 404 portal not found
6× debug/echo route probes                                          # all 404
4× DNS probes on atriumdev.app siblings                             # all NXDOMAIN (Status:3)
git clone --depth 1 … forge-atrium; read portal.go + blocks.go      # links 4,5
```

**Secret hygiene:** `forge_api_key` was fetched via `secret_get` to `/tmp/.forge_api_key` (0600), read only inside shell steps, never into my context, and wiped at end of smoke — `shred -u` verified (`wiped: /tmp/.forge_api_key`). No credential value appears anywhere in this handoff. I also scanned this draft for credential-shaped strings: the only long tokens are commit SHAs, a `cf-ray`, an `x-amzn-requestid`, and a `defaultAgentId` UUID — all non-secret identifiers.

## 6. Open items for the operator (not blockers on this verdict)

1. **CloudWatch Logs read** on the `forge-atrium` staging Lambda log group is the single grant that would turn AC3's host-keyed-response evidence into a literal log line. `cmd/atrium/main.go:1452-1459` already logs `"host", portal.Host(r)` — the artifact exists and is written; only read access is missing. The correlation ID to look up for my request is `x-amzn-requestid: e780e2ba-e1c8-497b-8ef4-1a23c5e01f2c`. I did **not** push this downstream: no character in this chain holds an AWS credential, so handing it on would burn a turn against the same wall.
2. **Test-coverage gap (real, out of scope here):** `pkg/clouds/pulumi/cloudflare/` has **no** test asserting the `X-Forwarded-Host` line, while the Yandex equivalent does (`yandex/registrar_test.go`). A silent regression in the worker template would ship undetected. Worth a roadmap item.
3. **Correction to file for the roadmap:** the developer's claim that `client.yaml:131` ("atriumdev.ru has no provisioned portals yet") is merely *"possibly stale"* should be recorded as **confirmed stale** — the live registry lists `.ru` as a registered domain.
4. **Obsolete doc comment** in `forge-atrium` `cmd/atrium/main.go:162-182` still describes the pre-fix world ("adds no X-Forwarded-Host"), now actively misleading.

Per the brief I did **not** touch `docs/roadmap/` in the `forge` repo — that bookkeeping belongs to the Roadmap Driver; items 2-4 are handed over as recommendations, not edits.

## Summary

- **Re-ran the verification independently** rather than ratifying the developer's transcript; reproduced the live 200 with my own fresh correlation IDs.
- **Closed the chain's one unverified link.** The developer's counterfactual ("a miss returns 404") and registration claim came from prior-run markdown docs — I established both live against the conductor registry, including a 404 for a lambda-url-shaped hostname. That upgrades AC3 from their self-described "strong inference" to a five-step deduction.
- **Corrected a developer finding:** `.ru` is a registered portal domain per live registry, so their "stale comment" explanation of the `.ru` 200 was wrong — the confounder that "nearly broke" their conclusion never existed.
- **Stated the residual honestly:** no literal log line was observed, no CloudWatch grant exists, and no echo endpoint exists (6 probed, all 404). The brief's DoD ("a log line, a debug endpoint, a header echo, **etc.** — not just 'it didn't error'") is met by a response whose *content* is host-keyed — materially stronger than a non-error.
- Zero source changes, consistent with AC6. Lint gate reported as configured-but-vacuous rather than silently dropped or falsely greened.

**Empty-PR-OK:** yes — the brief dispatched a live verification of an already-merged-and-released fix, AC6 explicitly forbids source changes, and the AC table carries real live evidence (URLs, status codes, response bodies, `cf-ray`/`x-amzn-requestid` correlation IDs, and live registry 200/404 pairs).

**Verdict:** signoff