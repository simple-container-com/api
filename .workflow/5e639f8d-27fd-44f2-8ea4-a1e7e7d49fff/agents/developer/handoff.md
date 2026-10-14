Committed and verified. Here is my handoff.

---

# Developer Handoff — sc-ingress-drops-visitor-host: live audit executed, result is an honest NEGATIVE

## 1. Commit + branch identity

- **Commit:** `1e1ddf7b9f` (full: `1e1ddf7b9f71cf108f6f65cce2435b8740d18134`)
- **Branch:** `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/5e639f8d-27fd-44f2-8ea4-a1e7e7d49fff`
- PR: engine auto-opens at handoff time.
- Verified the artifact is real, not just described: `git log -1 -- docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` → `1e1ddf7b9f ... simple-container-forge[bot]` (this run).

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` | +144/-0 | New audit doc: 16-domain probe table, methodology control, grant blockers, rejected hypothesis, scope exclusions |

No `pkg/`, `cmd/`, or `internal/` file touched — confirmed by `git diff --name-only main..HEAD -- pkg cmd internal` → empty.

## 3. The headline finding (read this before the AC table)

**The audit the brief asked for cannot be completed black-box, and that is a structural fact, not a tooling failure.**

`X-Forwarded-Host` is a *request* header — observable only at the origin, or in a response if the origin routes/echoes on it. An org-wide search returned **4** hits for the string; only one is a consuming service (`forge-conductor` `cloudsessions/handler.go:2602`), and it is both **auth-gated** (401 unauthenticated) and **prefers `h.publicCallbackBaseURL`** when set, only falling back to the header. Every other fleet service is one-hostname-per-stack and never reads it.

So on 15 of 16 domains, a 200 means "the service is up" — **not** "the visitor's Host reached the origin". Marking those PASS would be exactly the fabricated signoff the contract forbids. I recorded them as **UNVERIFIABLE**, which is distinct from FAIL.

I validated the probe shape itself against the known-good case so the negative isn't just my method failing:

```
GET https://atriumdev.app/api/v1/portal                                  → 200, host-keyed manifest
GET https://atriumdev.app/api/v1/portals/resolve?domain=nonexistent-...  → 404 {"error":"We could not find that."}
```

The method works where the header is consumed. It has no purchase on single-tenant services.

**Two corrections to upstream assumptions:**
- **The roadmap item is not in this repo.** `docs/roadmap/items/sc-ingress-drops-visitor-host.md` lives in **`simple-container-com/forge`** (read it, sha `ed9fb2f248`), not `simple-container-com/api` (this run's linked repo). Architect's inability to find it was correct; PM's claim to have read it in `api` was wrong about the repo. This triggers the **cross-repo guard** — see §6.
- **A tempting hypothesis, rejected.** `concierge`/`meet` `/health` return the same `revision` but different `surface` (`admin`/`public`) — which looks like host-keyed routing on one deployment. It isn't: `SURFACE: public` is a baked env var on a *separate* stack/Lambda (`.sc/stacks/forge-concierge-meet/client.yaml`). I nearly logged this as the AC2 control; it would have been a false pass. Recorded in the doc so nobody re-derives it.

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Live HTTP request per in-scope domain, result recorded verbatim | **PASS** | All 16 probed, 2026-10-14 17:20–17:29 UTC. 14 live (12× 200, `sessions`/`inference` 404 on `/` + 200 on `/health`, `storage` 401 auth-gated), all with `cf-ray` confirming Cloudflare in path. Full table in the committed doc. |
| AC2 | Negative/control case per domain that would fail if host routing weren't working | **FAIL-blocked** | Impossible black-box on 15/16 domains — no externally-reachable surface depends on XFH (only consumer is auth-gated + base-URL-preferring). Methodology control run on `atriumdev.app` instead (200 host-keyed + 404 negative, quoted above) to prove the probe shape is sound. Needs a **Cloudflare API token scoped to Workers-script read**. |
| AC3 | Pass/fail table committed into `items/sc-ingress-drops-visitor-host.md` `## Status`/`## History` | **FAIL-blocked** | That file is in `simple-container-com/forge`, NOT this run's linked repo (`api`). Per the cross-repo guard I did not push to another repo. Full table committed to `api` at the path in §2; the `forge` edit is named in §6. |
| AC4 | Name precisely any domain needing re-provision | **PARTIAL** | Cannot determine re-provision state without Pulumi/Cloudflare access. Did surface two concrete anomalies: `inference-test.simple-forge.com` (`198.18.140.62`) and `forge-test.simple-container.com` (`198.18.140.63`) complete TLS (`Verify return code: 0 (ok)`) then fail HTTP on :443 **and** :80. Retried **4×** over ~30s per policy — stable, while the other 14 succeeded in the same sweeps. Reported as observation, not diagnosed. |
| AC5 | Flip `status: shipped` if all pass; else say precisely what remains | **PASS** (correctly NOT shipped) | Did not flip. Evidence does not support fleet-wide shipped; the required grant is named in AC2. `status: in-progress` is correct. |
| AC6 | No `api` source file touched | **PASS** | `git diff --name-only main..HEAD -- pkg cmd internal` → empty. Docs-only. |

## 5. Tests run

No code mutated → no Go suite applies. `go build`/`go test` deliberately **skipped**: the diff is a single markdown file and cannot affect compilation. Verification was live HTTP + source reads:

```
$ for d in <16 domains>; do curl -sS -o /dev/null -w "%{http_code}" https://$d/; done
app 200 | ai 200 | sessions 404 | notifier 200 | storage 401 | inference 404
inference-test 000 | talk 200 | concierge 200 | meet 200 | vpr 200
forge.simple-container.com 200 | forge-test 000 | simple-container.com 200
simple-forge.com 200 | voice 200          (cf-ray present on all 14 reachable)

$ curl -sS https://api.cloudflare.com/client/v4/zones
403 {"success":false,"errors":[{"code":9106,"message":"Missing X-Auth-Email header"}]}

$ command -v aws; command -v sc        → neither present
$ env | grep -E 'CLOUDFLARE|AWS|CF_'   → no cloud credentials (names only; no values inspected)
```

Transient-retry trail: `talk`/`meet` returned `000` on one sweep, recovered on retry (both 200) — logged as blips, not findings. `inference-test`/`forge-test` retried 4× with identical `SSL_ERROR_SYSCALL` → escalated as real per policy. Direct `git clone` to github.com is blocked in this sandbox (`Failed to connect ... port 443`); used the MCP repo tools, which proxy correctly.

## 6. Cross-repo dependency — operator decision needed

**`cross_repo_dependency: simple-container-com/forge`**

AC3's deliverable (the audit table in `docs/roadmap/items/sc-ingress-drops-visitor-host.md`, plus a `## History` line) belongs in **`simple-container-com/forge`**, while this run's linked repo is **`simple-container-com/api`**. Per the cross-repo guard I committed only the `api`-side portion and did not push to `forge`. The `forge` edit needed:

- `## Status`: add a row — *"Fleet audit of other domains · ⚠️ inconclusive 2026-10-14 · 14/16 live and CF-fronted, but XFH is unobservable black-box (only consumer is auth-gated); needs a Cloudflare Workers-script read token. See `api` `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` @ `1e1ddf7b9f`."*
- Front matter: keep `status: in-progress`; set `blocked_on: "Cloudflare API token scoped to Workers-script read"`.
- `## History`: the 2026-10-14 entry, including the two unreachable `-test` domains.

## 7. Notes for QA / DevOps

- **Do not re-probe the 15 domains hoping for a different answer** — the limitation is the fleet's, not the probe's. The decisive move is one Cloudflare API call per domain: `GET /client/v4/accounts/{id}/workers/scripts/{name}`, grep for `fwdHeaders.set("X-Forwarded-Host", origHost)`. That inspects the exact per-domain artifact at issue, needs no service cooperation, and works uniformly for Lambda and static-site domains.
- QA holds `forge_api_key` (conductor), which is **not** the Cloudflare token — I don't believe this is resolvable downstream; it reads as a genuine operator grant request.
- `inference-test` / `forge-test` deserve an operator look independent of the header question — DNS outliving a torn-down stack is the likely shape, but I did not confirm it and did not guess a fix.
- `slice_id: sc-ingress-drops-visitor-host`

**Empty-PR-OK:** yes — read-only audit explicitly forbidden from changing `api` source; the whole diff is one docs file carrying live probe evidence, command output, and named grant blockers.

**Verdict:** signoff