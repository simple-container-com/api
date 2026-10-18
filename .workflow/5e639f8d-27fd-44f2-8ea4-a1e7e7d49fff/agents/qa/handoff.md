Verification complete. Here is my handoff.

---

# QA Handoff — sc-ingress-drops-visitor-host: developer's negative result VERIFIED, slice correctly not shippable

## 1. Commit + branch identity

- **Commit:** `2ec145ef0b` (full: `2ec145ef0bed`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/5e639f8d-27fd-44f2-8ea4-a1e7e7d49fff`
- Developer's prior commit independently verified as real, not described: `git log -1 -- docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` → `1e1ddf7b9f71cf10… simple-container-forge[bot]`, this run. **Substance gate satisfied** — a real SHA, a real non-`.workflow/` source path, with live command output.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` | +55/-2 | Added `## QA independent verification` — re-run sweep + `voice` correction, spoof-clobber discriminator result, the caught false pass, structural-claim recheck, dead-domain retries |

No `api` source touched: `git diff --name-only main..HEAD -- '*.go'` → empty.

## 3. I did not take the developer's word for it — what I re-ran

**The sweep reproduced, with one correction.** 14/16 codes matched exactly, `cf-ray` on all 14. **`voice.simple-forge.com` returned `000` on my first sweep, 200 on retry** — their 200 stands, but the domain is flappier than a single-sample table shows.

**I tried a discriminator the chain hadn't.** The fixed worker uses `set()`, not `append()`, so it must *clobber* a client-supplied `X-Forwarded-Host` — a candidate credential-free test. On the known-good domain it behaves exactly as the fix predicts (200 baseline, **byte-identical** under spoof, 404 negative control). But on `app` and `notifier` the spoofed and unspoofed bodies are also byte-identical — equally consistent with "worker clobbered it" and "origin never read it". **It cannot separate the two hypotheses**, so it yields no per-domain verdict. The developer's conclusion survives a genuine attempt to break it.

**A near-miss false pass, caught.** `simple-forge.com` *did* return a DIFFERENT body under spoof — briefly reading as the fleet's one real host-routing signal. It is **noise**: two *identical* no-spoof requests also differ (`cmp` → byte 47698 of 56066, same length both times — a per-render nonce). Logging that as AC2 evidence would have been a fabricated pass. Recorded in the doc so nobody re-derives it.

**The structural claim holds even with a credential the developer lacked.** Exactly 4 org-wide hits; one consumer. I read the call site directly: the value flows into `serviceURL` on a **worker-dispatch payload** (`cloudexec.TurnDispatch`), never into an HTTP response body, and `publicCallbackBaseURL` is preferred when set. Not observable black-box **even with** the conductor bearer (`forge_api_key`) I hold. The missing grant is specifically a **Cloudflare token** — nothing I hold substitutes.

**Upstream corrections confirmed.** The roadmap item is in `simple-container-com/forge` (read it, sha `ed9fb2f248`, `status: in-progress`), **not** `api`. Developer was right; **PM's claim to have read it in `api` was wrong about the repo** — treat that handoff's file citations as unverified.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Live HTTP request per in-scope domain, recorded verbatim | **PASS** | Independently re-swept all 16 this turn; output in §3 and committed doc. 14/16 matched developer's table; `voice` corrected to a transient. |
| AC2 | Negative/control case per domain that would fail if host routing weren't working | **FAIL-blocked** | Not achievable black-box. I tried the remaining credential-free discriminator (spoof-clobber); works on `atriumdev.app`, **inconclusive by construction** on single-tenant origins. Needs **Cloudflare API token scoped to Workers-script read**. |
| AC3 | Table committed into `items/sc-ingress-drops-visitor-host.md` `## Status`/`## History` | **FAIL-blocked** | That file is in `simple-container-com/forge`; this run's linked repo is `api`. Cross-repo guard respected — not pushed to another repo. Exact `forge` edit specified in §6. |
| AC4 | Name precisely any domain needing re-provision | **PARTIAL** | Undeterminable without Pulumi/Cloudflare access. Confirmed two stable anomalies: `inference-test` (`198.18.140.62`) and `forge-test` (`198.18.140.63`) — `000` on **3/3 retries each** while 14 peers succeeded in the same sweeps. Observation only; no fix guessed. |
| AC5 | Flip `status: shipped` if all pass; else state precisely what remains | **PASS** (correctly NOT shipped) | Not flipped. `status: in-progress` + named Cloudflare-token blocker is the accurate state. |
| AC6 | No `api` source file touched | **PASS** | `git diff --name-only main..HEAD -- '*.go'` → empty. Docs-only. |

## 5. Tests run

Build/test/lint gates assessed and **correctly inapplicable** — the diff is one markdown file, zero `.go` files (`git diff --name-only main..HEAD -- '*.go'` → empty), so it cannot affect compilation or lint. Per my standing static-lint rule I checked anyway: the repo **does** carry `.golangci.yml`, but neither `golangci-lint`/`bin/golangci-lint` nor the **Go toolchain itself** is installed in this sandbox (`command -v go` → not found). I am not claiming a lint PASS I did not observe; with no Go delta the gate is a genuine no-op here.

```
# independent sweep (16 domains)
app 200 | ai 200 | sessions 404 | notifier 200 | storage 401 | inference 404
inference-test 000 | talk 200 | concierge 200 | meet 200 | vpr 200
forge.simple-container.com 200 | forge-test 000 | simple-container.com 200
simple-forge.com 200 | voice 000→200 on retry        (cf-ray on all 14 reachable)

# spoof-clobber on known-good
atriumdev.app /api/v1/portal            → 200  {"brand":{"name":"Atrium",...}}
  ″  + X-Forwarded-Host: bogus…         → 200  byte-identical  → clobbered (fixed worker)
atriumdev.app /portals/resolve?domain=… → 404  {"error":"We could not find that."}

# false-pass check on simple-forge.com
two identical no-spoof requests → UNSTABLE, differ at byte 47698/56066 → earlier DIFFERENT = noise

# retries, 3× each: inference-test 000/000/000 | forge-test 000/000/000
```

## 6. Operator action required — two distinct items

**(a) Grant blocker — the decisive move.** One Cloudflare API token scoped to Workers-script read turns this 16-domain unverifiable audit into a deterministic one in a single pass: `GET /client/v4/accounts/{id}/workers/scripts/{name}`, grep for `fwdHeaders.set("X-Forwarded-Host", origHost)`. Inspects the exact per-domain artifact at issue, needs no service cooperation, works uniformly for Lambda and static-site domains. **Not resolvable by any downstream character** — I verified the conductor bearer doesn't help.

**(b) Cross-repo edit — `cross_repo_dependency: simple-container-com/forge`.** AC3's deliverable belongs in `forge`; I did not push outside the linked repo. Needed there: `## Status` row — *"Fleet audit of other domains · ⚠️ inconclusive 2026-10-14 · 14/16 live and CF-fronted; XFH unobservable black-box (sole consumer is auth-gated, writes to a dispatch payload not a response, prefers publicCallbackBaseURL); spoof-clobber discriminator inconclusive on single-tenant origins. Needs Cloudflare Workers-script read token. See `api` `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` @ `2ec145ef0b`."*; keep `status: in-progress`; set `blocked_on: "Cloudflare API token scoped to Workers-script read"`; add the 2026-10-14 History entry incl. the two dead `-test` domains.

**(c) Independent of the header question:** `inference-test` / `forge-test` warrant an operator look — DNS outliving a torn-down stack is the likely shape, but I did not confirm it.

## 7. QA ruling

The developer did the harder, more honest thing: they ran the audit, found it structurally impossible black-box, and classified 15/16 as **UNVERIFIABLE** rather than banking 200s as PASS. That classification is **correct and verified** — I attempted to break it with a new discriminator and could not. UNVERIFIABLE is distinct from FAIL and must not be upgraded to PASS by a downstream turn.

Gate A/B (isolated-stack deploy/smoke) are **not applicable**: this slice audits standing fleet domains, and an ephemeral `<wf_id>--*.simple-forge.com` stack would only mint *new* workers, proving nothing about existing ones. No live-gate URLs are claimed.

I sign off on **the audit turn's quality and honesty**, not on the slice being shipped — those are different things, and the roadmap item stays `in-progress`.

**Empty-PR-OK:** yes — read-only audit explicitly forbidden from changing `api` source; the whole non-`.workflow/` diff is one docs file carrying live probe output, a reproduced sweep, a caught false pass, and named grant blockers.

**Verdict:** signoff