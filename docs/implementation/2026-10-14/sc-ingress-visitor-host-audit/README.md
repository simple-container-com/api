# `X-Forwarded-Host` fleet audit — per-domain Cloudflare worker

**Slice:** `sc-ingress-drops-visitor-host`
**Date:** 2026-10-14
**Type:** read-only audit. No `api` source change was made or intended.

## Why this audit exists

`NewOverrideHeaderRule` (`pkg/clouds/pulumi/cloudflare/registrar.go`) mints a **separate**
Cloudflare worker script per SC-provisioned domain, with `overrideHost` baked in as a string
literal via `sdk.Sprintf`. The `X-Forwarded-Host` fix (PR #406, merged `5d6d0afe`, released in
SC 2026.9.16) is therefore **not** fleet-wide on merge: each domain picks it up only when its own
registrar leg re-provisions on that stack's next deploy.

`atriumdev.app` has been live-verified. This audit asked whether the other SC-provisioned domains
carry the same fixed worker.

## Headline result — the audit is inconclusive by construction, and that is the finding

**No in-scope domain other than `atriumdev.app` can be audited from outside.** This is not a
tooling gap that a retry or a different probe would close; it is a property of the fleet:

1. `X-Forwarded-Host` is a **request** header. It is observable only at the origin — in the
   response only if the origin chooses to echo or route on it.
2. An org-wide code search for `X-Forwarded-Host` returns **four** hits, and only one is a
   service that consumes it: `forge-conductor`'s
   `internal/service/handlers/cloudsessions/handler.go`. The other three are the SC registrar
   codegen itself plus its Yandex test. Every other fleet service is one-hostname-per-stack and
   never reads the header — so no externally-reachable response varies with it.
3. The single consumer is not externally observable either. It sits behind authentication
   (`/api/v1/cloud-sessions/*` → 401 unauthenticated) **and** prefers `h.publicCallbackBaseURL`
   when configured, only falling back to the header. A 401 proves nothing about the header.

A green positive case on these domains would mean "the service is up", not "the visitor's Host
reached the origin". Recording such a result as PASS would be a fabricated signoff.

## What a conclusive audit requires

One of the following, none of which this run holds:

| Method | Blocker |
|---|---|
| Read each minted worker script's source and confirm it contains the `fwdHeaders.set("X-Forwarded-Host", origHost)` line | **Cloudflare API token** (`GET /client/v4/accounts/{id}/workers/scripts/{name}`). Unauthenticated probe returns 403 `9106 Missing X-Auth-Email header`. |
| Read the origin's received headers from logs | **AWS credentials + CloudWatch Logs read.** No `aws` CLI and no credential material present in this environment. |
| Check each stack's last-deploy timestamp against SC 2026.9.16 (2026-09-30) | **Pulumi state access** (`sc stack export` / backend creds). No `sc` CLI present. |

The Cloudflare-API route is the cheapest and the most direct: it inspects the exact artifact whose
per-domain-ness is the whole problem, needs no service cooperation, and works uniformly for
Lambda-backed and static-site domains.

## Live probe results (what *was* established)

All requests made 2026-10-14 ~17:20–17:29 UTC from the run sandbox. `cf_ray` present confirms the
request genuinely traversed Cloudflare (sandbox egress NATs through `198.18.0.0/15`, which is
*not* Cloudflare address space — the `cf-ray` header and a valid Let's Encrypt cert for
`CN = simple-forge.com` are what establish Cloudflare is in path).

| # | Domain | `GET /` | cf-ray | XFH verdict |
|---|---|---|---|---|
| 1 | `app.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 2 | `ai.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 3 | `sessions.simple-forge.com` | 404 (`/health` → 200) | yes | **UNVERIFIABLE** |
| 4 | `notifier.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 5 | `storage.simple-forge.com` | 401 (auth-gated) | yes | **UNVERIFIABLE** |
| 6 | `inference.simple-forge.com` | 404 (`/health` → 200) | yes | **UNVERIFIABLE** |
| 7 | `inference-test.simple-forge.com` | TLS handshake OK, HTTP never completes | n/a | **UNREACHABLE** — see below |
| 8 | `talk.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 9 | `concierge.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 10 | `meet.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 11 | `vpr.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 12 | `forge.simple-container.com` | 200 | yes | **UNVERIFIABLE** |
| 13 | `forge-test.simple-container.com` | TLS handshake OK, HTTP never completes | n/a | **UNREACHABLE** — see below |
| 14 | `simple-container.com` | 200 | yes | **UNVERIFIABLE** |
| 15 | `simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| 16 | `voice.simple-forge.com` | 200 | yes | **UNVERIFIABLE** |
| — | `atriumdev.app` (methodology control) | 200, host-keyed manifest | yes | **PASS** (pre-existing) |

`UNVERIFIABLE` = the domain is live and Cloudflare-fronted, but nothing in its externally-reachable
surface depends on `X-Forwarded-Host`, so no black-box request can distinguish a fixed worker from
an unfixed one. It is **not** a failure, and must not be read as one.

### Methodology control — `atriumdev.app`

Re-run to prove the probe shape itself works where the header *is* consumed:

```
$ curl -s https://atriumdev.app/api/v1/portal
200 {"brand":{"name":"Atrium","colors":{"accent":"#6d5cff"}},"blocks":["sign-in","chat",
     "processes"],"assistantName":"Atrium Assistant","chatTransport":"sse"}

$ curl -s "https://atriumdev.app/api/v1/portals/resolve?domain=nonexistent-bogus.example.com"
404 {"error":"We could not find that."}
```

Positive is host-keyed; same-origin negative control 404s. The audit method is sound — it simply
has no purchase on single-tenant services.

### The two unreachable domains

`inference-test.simple-forge.com` (`198.18.140.62`) and `forge-test.simple-container.com`
(`198.18.140.63`) resolve, complete a TLS handshake with `Verify return code: 0 (ok)`, and then
fail the HTTP exchange with `SSL_ERROR_SYSCALL` on both :443 and :80. Retried **4×** across ~30s
per the run's transient-retry policy; identical every time, and the *other* 14 domains succeeded in
the same sweeps. This is a stable condition, not a network blip.

Both are `-test` stacks. The most likely explanation is a torn-down or never-provisioned test
stack whose DNS record outlives its origin, but **this run cannot confirm that** without Pulumi
state access. Recorded as an observation, not a diagnosis. It needs an operator eye regardless of
the header question.

## A rejected hypothesis, recorded so it is not re-derived

`concierge.simple-forge.com/health` and `meet.simple-forge.com/health` return the **same**
`service` and `revision` but different `surface` values (`admin` vs `public`) — which looks like
host-keyed routing on one deployment, i.e. exactly the Atrium-shaped control this audit wanted.

**It is not.** `.sc/stacks/forge-concierge-meet/client.yaml` sets `SURFACE: public` as a baked
build-time env var on a **separate** stack and a separate Lambda. The two surfaces differ because
they are two deployments, not because either reads the visitor's Host. Spoofing
`X-Forwarded-Host: meet.simple-forge.com` at `concierge` correctly did **not** change the response
— consistent both with the worker's `set()` clobbering a client-supplied value *and* with the
service simply never reading it. The test cannot separate those two, so it proves nothing either
way.

## Scope note — mechanisms deliberately excluded

Only the Cloudflare-Worker mechanism is in scope. Two other mechanisms serve fleet domains and are
**unaffected** by PR #406, on code-path grounds (not staleness):

- **ECS Fargate → ALB** (`baas.simple-forge.com`, `meet-baas.simple-forge.com`): `ecs_fargate.go`
  calls `Registrar.NewRecord` directly and never `ProvisionDomainForEndpoint`/
  `NewOverrideHeaderRule`. No worker is minted; no Host rewrite happens, so there is nothing to
  forward around.
- **Yandex Cloud API Gateway** (`atriumdev.ru`, `talk.atriumdev.ru`): `yandex/registrar.go` maps
  `X-Forwarded-Host: '{Host}'` in the gateway's own OpenAPI spec, asserted by
  `yandex/registrar_test.go`. Correct by construction, independent of the Cloudflare worker.

Auditing these would produce a false signal in either direction.

## QA independent verification (2026-10-14, QA turn)

QA re-ran the sweep rather than accepting the table above on trust. Results and corrections:

**1. Probe sweep reproduced, with one correction.** 14 of 16 domains matched the developer's
recorded codes exactly (`app` 200, `ai` 200, `sessions` 404, `notifier` 200, `storage` 401,
`inference` 404, `talk` 200, `concierge` 200, `meet` 200, `vpr` 200,
`forge.simple-container.com` 200, `simple-container.com` 200, `simple-forge.com` 200, `voice` 200),
`cf-ray` present on all 14. **Correction:** `voice.simple-forge.com` returned `000` on QA's first
sweep and `200` on retry 1 — a transient, so the developer's 200 stands, but the domain is flappier
than a single-sample table shows.

**2. A new discriminator was tried, and it does not rescue the audit.** The fixed worker uses
`set()`, not `append()`, so it must **clobber** a client-supplied `X-Forwarded-Host`. That makes
"spoof the header and see whether the origin reacts" a candidate black-box test needing no
credentials. On the known-good domain it behaves exactly as the fix predicts:

```
GET https://atriumdev.app/api/v1/portal                                   → 200 host-keyed manifest
GET  ″  with -H "X-Forwarded-Host: nonexistent-bogus-host.example.com"    → 200, byte-identical
GET https://atriumdev.app/api/v1/portals/resolve?domain=nonexistent-...   → 404 {"error":"We could not find that."}
```

Byte-identical under spoof = the worker replaced the client value. But on single-tenant domains the
test is **inconclusive by construction**: `app` and `notifier` returned byte-identical responses
spoofed vs. unspoofed, which is equally consistent with "worker clobbered it" and with "origin never
read it". It cannot separate the two hypotheses, so it yields no per-domain verdict.

**3. A near-miss false pass, caught.** `simple-forge.com` *did* return a DIFFERENT body under spoof
— briefly reading as the fleet's one genuine host-routing signal. It is **noise**: two *identical*
requests with no spoof at all also differ (`cmp` → first difference at byte 47698 of 56066, same
total length both times — a per-render nonce in the static bundle). Logging that DIFFERENT as AC2
evidence would have been a fabricated pass. Recorded so the next reader does not re-derive it.

**4. The structural claim holds under independent check.** QA re-ran the org-wide search: exactly
**4** hits for `X-Forwarded-Host`, only one a consumer (`forge-conductor`
`cloudsessions/handler.go:2602`). QA read that call site directly — the resolved value flows into
`serviceURL` on a **worker-dispatch payload** (`cloudexec.TurnDispatch`), never into an HTTP
response body, and `h.publicCallbackBaseURL` is preferred when set. So it is not observable
black-box even **with** the conductor bearer (`forge_api_key`) that QA holds and the developer did
not. The missing grant is specifically a **Cloudflare token**; no credential QA holds substitutes.

**5. The two dead `-test` domains are stable, not transient.** Retried 3× each, `000` every time,
while 14 peers succeeded in the same sweeps. Both resolve to sandbox-NAT addresses adjacent to the
live ones (`inference-test` → `198.18.140.62`, `forge-test` → `198.18.140.63`, vs. live `inference`
→ `198.18.78.99`). Reported as an observation for the operator; not diagnosed, no fix guessed.

**QA verdict on the slice:** the developer's `UNVERIFIABLE` classification is correct and is the
right call — it is distinct from FAIL and must not be upgraded to PASS. `status: in-progress` with
a named Cloudflare-token blocker is the accurate state.

## Bottom line

The slice is **not** shippable on this evidence, and no amount of further black-box probing will
make it so — QA tried the one remaining credential-free discriminator (spoof-clobber) and it is
inconclusive on single-tenant origins by construction. The honest next step is a Cloudflare API
token scoped to Workers-script read, which turns a 16-domain unverifiable audit into a 16-domain
deterministic one in a single pass.
