[thinking] # PM Handoff — sc-ingress-drops-visitor-host: Audit Scope & Plan

## 0. Context digested

Read `docs/roadmap/items/sc-ingress-drops-visitor-host.md` (full History), `pkg/clouds/pulumi/cloudflare/registrar.go` (the actual worker codegen — `NewOverrideHeaderRule`), `pkg/clouds/pulumi/registrars.go` (multi-registrar dispatch by zone), `pkg/clouds/pulumi/yandex/registrar.go` (the YC equivalent), `pkg/clouds/pulumi/aws/ecs_fargate.go` (how Fargate stacks get a DNS record), and every `.sc/stacks/*/client.yaml` I could reach across the fleet's onboarded repos (forge, forge-conductor, forge-aigateway, forge-sessions, forge-notifier, forge-storage, forge-baas, forge-runtime, forge-atrium, forge-talk, forge-concierge, landing, vpr-math-tutor). This is a **PM planning turn** — I did not make any live HTTP requests; that is explicitly QA/Developer's job per the brief ("make a live HTTP request… Record which domains pass and which don't"). My job is to hand Architect a bounded, correct scope so the SDLC doesn't waste turns auditing domains the fix never touches.

## 1. Commit + branch identity

No code mutations in this turn — PM role produces a plan only. The roadmap-item documentation update (Definition of Done requires writing the audit table into `items/sc-ingress-drops-visitor-host.md`) is explicitly QA/DevOps's deliverable once the live evidence exists, not mine.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none (PM planning turn) |

## 3. Requirements, restated

Audit every **other** SC-provisioned domain's Cloudflare per-domain worker (the `NewOverrideHeaderRule` script, `pkg/clouds/pulumi/cloudflare/registrar.go`) for the same `X-Forwarded-Host` behaviour live-proven on `atriumdev.app`. Read-only — no `api` source change. Record pass/fail per domain with the exact request used, committed into the item's own `## Status`/`## History`.

## 4. Scope — critical correction before dispatch

**The worker mechanism in question (`NewOverrideHeaderRule`) is NOT how every SC-provisioned domain is served.** I traced the actual provisioning code paths; there are **three distinct mechanisms** in the fleet, and only one of them is the code PR #406 touched:

| Mechanism | Code path | Forwards visitor Host how | PR #406 relevant? |
|---|---|---|---|
| **Cloudflare Worker, per-domain** (`NewOverrideHeaderRule`) | `aws_lambda.go` → `Registrar.ProvisionDomainForEndpoint` → cloudflare registrar, for every AWS Lambda `single-image`/`lambda` stack behind `baseDnsZone: simple-forge.com` / `simple-container.com`. Also static sites (`aws/bucket.go`, `gcp/static_website.go`) call `NewOverrideHeaderRule` directly. | Worker script sets `X-Forwarded-Host` to `origHost` — **this is the exact code PR #406 fixed.** | **YES — in scope** |
| **ECS Fargate → ALB, plain CNAME** | `ecs_fargate.go` calls `params.Registrar.NewRecord` directly (proxied CNAME to the ALB DNS name) — **never calls `NewOverrideHeaderRule` or `ProvisionDomainForEndpoint`.** No worker script is minted for these stacks at all. | Cloudflare's default proxied-record passthrough to the ALB; the ALB/target already receives the visitor's real Host over TLS (no hostname rewrite happens here in the first place — there is nothing to forward around). | **NO — different code, unaffected by this bug or fix** |
| **Yandex Cloud, API Gateway** | `yandex/registrar.go` → `ProvisionDomainForEndpoint` builds an API Gateway OpenAPI spec that explicitly maps `Host → target's own hostname` and `X-Forwarded-Host → visitor's hostname` **in the gateway spec itself** (see `proxySpecFor`'s long comment — this was built correctly from day one, independently of the Cloudflare worker). | Gateway-level header mapping, not a Cloudflare Worker. | **NO — separate mechanism, already correct by construction, not part of this incident** |

Treating ECS-Fargate or YC domains as part of "every other SC-provisioned domain" audit would produce a false signal either way (a pass there proves nothing about the Worker fix; a fail there would wrongly implicate PR #406). **Scope this audit strictly to domains served through the Cloudflare-Worker mechanism.**

### In-scope domain list (Cloudflare-Worker mechanism, excluding the already-proven `atriumdev.app`)

Enumerated from every `.sc/stacks/*/client.yaml` reachable in onboarded repos as of 2026-10-14:

| # | Domain | Repo / stack | Template |
|---|---|---|---|
| 1 | `app.simple-forge.com` | forge-conductor | lambda |
| 2 | `ai.simple-forge.com` | forge-aigateway | lambda |
| 3 | `sessions.simple-forge.com` | forge-sessions | lambda |
| 4 | `notifier.simple-forge.com` | forge-notifier | lambda |
| 5 | `storage.simple-forge.com` | forge-storage | lambda |
| 6 | `inference.simple-forge.com` | forge-runtime (`runtime-lambda`, staging) | lambda |
| 7 | `inference-test.simple-forge.com` | forge-runtime (`runtime-lambda`, test) | lambda |
| 8 | `talk.simple-forge.com` | forge-talk | lambda |
| 9 | `concierge.simple-forge.com` | forge-concierge | lambda |
| 10 | `meet.simple-forge.com` | forge-concierge-meet | lambda |
| 11 | `vpr.simple-forge.com` | vpr-math-tutor | lambda |
| 12 | `forge.simple-container.com` | forge (legacy `cmd/`, staging) | lambda |
| 13 | `forge-test.simple-container.com` | forge (legacy `cmd/`, test) | lambda |
| 14 | `simple-container.com` | landing (`landing` stack) | static-site (S3+CF) |
| 15 | `simple-forge.com` | landing (`forge-landing` stack) | static-site (S3+CF) |
| 16 | `voice.simple-forge.com` | landing (`voice-landing` stack) | static-site (S3+CF) |

### Explicitly OUT of scope (name precisely, don't silently skip)

| Domain | Why excluded |
|---|---|
| `baas.simple-forge.com` (forge-baas) | ECS Fargate, plain CNAME to ALB — no Worker minted |
| `meet-baas.simple-forge.com` (baas-meeting) | same — ECS Fargate |
| `atriumdev.ru` (forge-atrium staging-ru) | Yandex API Gateway — already forwards `X-Forwarded-Host` by its own OpenAPI spec, unrelated code path |
| `talk.atriumdev.ru` (forge-talk staging-ru) | same — YC |
| Any per-run isolated-stack subdomain (`<wf_id>--*.simple-forge.com`) | ephemeral, destroyed after the run; not a standing fleet domain worth a permanent roadmap row |

## 5. MoSCoW

| Item | Priority |
|---|---|
| Live request + evidence for the 16 in-scope Cloudflare-Worker domains | **MUST** |
| Explicit scope note (ECS-Fargate / YC excluded, with reasoning) in the item file | **MUST** — prevents a future reader from reading "audited every domain" as literal |
| A reusable/documented test method per service shape (single-tenant vs multi-tenant) | **MUST** |
| Re-testing the already-proven `atriumdev.app` | **CUT** — already live-verified 5× independently per History |
| Auditing ECS-Fargate / YC domains for the same bug | **CUT** — different code, see §4 |
| Any `api` source change | **CUT** — brief explicitly forbids it; branch-protection blocks bot merge anyway |

## 6. Acceptance criteria for this slice (for Architect/Developer/QA)

| AC# | Criterion |
|---|---|
| AC1 | For each of the 16 in-scope domains, a live HTTP request is made against the real deployed hostname and its result (status, relevant body/header evidence) is recorded verbatim. |
| AC2 | Each domain's evidence includes a **negative/control** case that would fail if host-based routing weren't functioning — see §7 for the two acceptable control shapes, since most of these services are single-tenant (unlike Atrium) and the Atrium-style registry-miss 404 control doesn't apply verbatim. |
| AC3 | Pass/fail is recorded per domain in a table, committed into `items/sc-ingress-drops-visitor-host.md`'s `## Status` (or a new `## Audit` sub-section) and `## History` — not left only in the handoff. |
| AC4 | If any domain needs its own re-provision event to pick up the fix (e.g. a stack whose last deploy predates SC 2026.9.16 / commit `5d6d0afe`), name that domain and what's missing precisely — do not guess a fix. |
| AC5 | If all 16 pass, flip `status: shipped` in the front-matter with the evidence table as the shipped-proof; if not, leave `status: in-progress` (or `blocked` if an operator action is needed) and say exactly which domain(s) remain and why. |
| AC6 | No `api` source file is touched. |

## 7. Test-methodology guidance for Architect/Developer/QA

The item's own text already flags *why* this bug was invisible for a year: **every service in the fleet except Atrium is one-hostname-per-stack**, so most of these 16 services don't consume `X-Forwarded-Host` at all today (they read the `HOST` env var or don't care). That means the Atrium-style "two registered domains behave differently, one unregistered domain 404s" control **does not transplant verbatim** — there's no second hostname on these stacks to distinguish. Two acceptable control shapes, pick whichever fits the service:

1. **Direct-vs-proxied diff (works for any of the 16).** Hit the public domain through Cloudflare (`https://<domain>/...`) and separately hit the raw Lambda Function URL directly (bypassing Cloudflare — the function URL is visible in the stack's Pulumi export / AWS console). A pass is: the Cloudflare-fronted request succeeds normally, **and** inspecting the request that reaches the origin (via an echo/debug endpoint if the service has one, or via CloudWatch/access logs showing the `X-Forwarded-Host` header value) shows the *visitor's* hostname, not the Lambda's internal one. This is the most literal transplant of the `atriumdev.app` proof.
2. **Header-echo endpoint, where one exists.** Several of these services run `go-aws-lambda-sdk`, which may log or expose request headers on a debug/health path — check for one before inventing a new probe.

For the **static sites** (14-16), the worker is the `OverridePages`-flavoured script in the same `NewOverrideHeaderRule` function — same `X-Forwarded-Host` line, so the same direct-vs-proxied diff applies against the S3 bucket's origin domain instead of a Lambda Function URL.

Do **not** invent a synthetic multi-tenant test on a single-tenant service to force an Atrium-shaped control — that would prove something these services don't actually rely on and isn't what "pass" should mean here. The honest pass criterion for a single-tenant service is "the worker correctly forwards the real Host to the origin," evidenced directly, not "swapping Host changes routing" (nothing routes on it).

## 8. Handoff to Architect

- Confirm my mechanism split in §4 against the live Cloudflare dashboard/Pulumi state for one or two stacks before dispatching Developer/QA on all 16 — if I've mis-traced something (e.g. a stack I read as `lambda` actually got migrated to `ecs-fargate` since this snapshot), adjust the in-scope list accordingly, but keep the ECS-Fargate/YC exclusion *reasoning* (it's a code-path argument, not a staleness one).
- Developer/QA: produce the live evidence per AC1-AC2, using whichever credentials/access (AWS console, Cloudflare API, CloudWatch) the run's grants provide. If a required grant (e.g. Cloudflare API token, AWS log read) isn't held by this run, stop at that domain and report the specific missing grant — do not guess.
- Whoever has repo-write in this chain: update `docs/roadmap/items/sc-ingress-drops-visitor-host.md` per AC3-AC5 before handoff to DevOps; this is a **documentation-only** deliverable (`sc-ingress-drops-visitor-host` already dispatched with no `api` source change expected), so `**Empty-PR-OK:** yes` is likely the correct declaration once live evidence is gathered and only `docs/roadmap/` changes.
- `slice_id: sc-ingress-drops-visitor-host` for downstream resolution.

**Verdict:** signoff