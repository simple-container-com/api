# DevOps provisioning-side audit — `sc-ingress-drops-visitor-host`

**Slice:** `sc-ingress-drops-visitor-host`
**Date:** 2026-10-14
**Companion to:** `./README.md` (developer + QA black-box probe audit)
**Type:** read-only. No `api` source change.

The developer and QA established — correctly — that `X-Forwarded-Host` is **not observable
black-box** on 15 of 16 in-scope domains, and that closing the audit the way the brief imagined
needs a Cloudflare Workers-script read token. I did not re-probe; re-running a probe that is
inconclusive by construction produces nothing.

Instead I audited the **provisioning side**, which is credential-free and answers a question the
black-box probe cannot: *which domains are even eligible to carry the fixed worker, and which
aren't provisioned at all.* Three findings, two of which correct the record.

---

## Finding 1 (correction) — the fix's commit SHA on `main` is `5252819589`, not `5d6d0afe`

The brief, the roadmap item and both prior handoffs cite the fix as "merged `5d6d0afe`". That
commit object exists, but **it is not reachable from `main`**:

```
GET /repos/simple-container-com/api/compare/2026.9.16...5d6d0afe3fee
→ 200, merge_base_commit.sha = eeaba3c0b93d
```

`eeaba3c0` is `5d6d0afe`'s own **parent**. A merge-base equal to the parent means the two refs
diverged there — `5d6d0afe` is a pre-merge (pre-squash) commit object, not an ancestor of the
release tag.

The commit that actually carries the change on `main`:

```
GET /repos/simple-container-com/api/commits?path=pkg/clouds/pulumi/cloudflare/registrar.go&sha=main
→ 5252819589f8
  "fix(cloudflare): forward the visitor's Host as X-Forwarded-Host (#406)"
  2026-09-23T19:05:52Z, PGP verified=true

GET /repos/simple-container-com/api/compare/5252819589f8...2026.9.16
→ 200, merge_base_commit.sha = 5252819589f8
```

Here the merge-base **equals the base commit**, which is exactly the ancestry proof: `5252819589`
is an ancestor of tag `2026.9.16`.

Both commits share tree `96c274084585`, so the *content* of the fix is
identical and nothing about the fix itself is in doubt. Only the citation is wrong. It matters
because `5d6d0afe` is the SHA an operator would use to bisect "did this deploy include the fix" —
and it resolves to a commit not on the release line.

**Use `5252819589f8` in the roadmap item and anywhere else the fix is
cited.**

## Finding 2 (confirmation) — SC `2026.9.16` genuinely contains the fix

```
GET /repos/simple-container-com/api/releases/tags/2026.9.16
→ tag_name 2026.9.16, target_commitish main,
  published_at 2026-09-30T10:54:52Z,
  tag commit 0aaefb044c65 (2026-09-30T10:33:17Z)
```

Combined with Finding 1's ancestry proof: the fix (merged 2026-09-23) is in the tree released as
`2026.9.16` (published 2026-09-30). The brief's release claim is **correct as stated**.

So the cut-over date for "an SC binary that mints the fixed worker" is **2026-09-30** for a
release-pinned CI, or **2026-09-23** for CI tracking `main`.

## Finding 3 (correction, material) — the two "unreachable" domains have NO DNS RECORD

The developer and QA both recorded `inference-test.simple-forge.com` → `198.18.140.62` and
`forge-test.simple-container.com` → `198.18.140.63`, TLS completing then HTTP failing, and reasoned
toward "DNS outliving a torn-down stack". I checked those names against two **public** resolvers
from outside the run sandbox:

```
GET https://dns.google/resolve?name=inference-test.simple-forge.com&type=A
→ {"Status":3, ... "Authority":[{"name":"simple-forge.com","type":6,
    "data":"bradley.ns.cloudflare.com. dns.cloudflare.com. ..."}]}        # Status 3 = NXDOMAIN

GET https://cloudflare-dns.com/dns-query?name=inference-test.simple-forge.com&type=A
→ {"Status":3, ...}                                                       # NXDOMAIN, confirmed

GET https://dns.google/resolve?name=forge-test.simple-container.com&type=A     → Status 0, NO Answer
GET https://cloudflare-dns.com/dns-query?name=forge-test.simple-container.com  → Status 0, NO Answer
GET https://dns.google/resolve?name=forge-test.simple-container.com&type=AAAA  → Status 0, NO Answer
GET https://dns.google/resolve?name=forge-test.simple-container.com&type=CNAME  → Status 0, NO Answer
GET https://dns.google/resolve?name=forge-test.simple-container.com&type=TXT    → Status 0, NO Answer
```

`inference-test` is **NXDOMAIN** on both resolvers. `forge-test` is **NODATA** — NOERROR with an
SOA in the authority section and no answer for A, AAAA, CNAME or TXT, i.e. the name carries no
address record of any kind.

Contrast with their live siblings, which answer with real Cloudflare anycast addresses:

```
inference.simple-forge.com   → 172.67.165.92, 104.21.91.30
forge.simple-container.com   → 104.21.34.207, 172.67.208.99
```

**The `198.18.140.x` addresses were sandbox artifacts, not real records.** `198.18.0.0/15` is
RFC 2544 benchmarking space; the run sandbox's DNS-NAT fabric synthesises an address in that range
for a name it cannot resolve, which is also why TLS appeared to "complete" and HTTP then died with
`SSL_ERROR_SYSCALL` — the handshake was against the NAT, not against Cloudflare. Note the audit
doc already observed that sandbox egress NATs through `198.18.0.0/15`; the same range appearing in
a *resolution* result is the tell.

### Why this changes the verdict rather than just the explanation

- These two domains are **not provisioned**. There is no Cloudflare DNS record, therefore no
  proxied CNAME, therefore **no minted worker script** — there is no artifact to audit.
- The correct audit verdict for them is **N/A — not provisioned**, not `UNREACHABLE`, and
  emphatically not FAIL.
- They need **no re-provision event for the header fix**. If anyone wants those `-test` stacks to
  exist, that is an ordinary "stack was never deployed / was destroyed" question, entirely
  separate from this slice. The AC4 answer is therefore *"no domain needs a re-provision event
  because of the fix; two domains do not exist"* — not *"two domains are broken"*.
- **The in-scope standing-fleet domain count is 14, not 16.**

## Deploy-window screen — a bound on risk, explicitly NOT a pass

PR #406's own blast-radius note states it plainly: *"every stack must redeploy to pick up the new
script."* So a domain's worker is fixed iff its stack has re-provisioned with SC ≥ the cut-over in
Finding 2.

I cannot read deploy history for every stack without Pulumi/Cloudflare access, but repository
activity is a **necessary condition** proxy in a deploy-on-push fleet, and it bounds the risk:

| Repo (stack owner) | Last default-branch activity | Cut-over 2026-09-30 |
|---|---|---|
| forge-conductor (`app`) | 2026-10-14 | after |
| forge-aigateway (`ai`) | 2026-10-14 | after |
| forge (`forge.simple-container.com`) | 2026-10-14 | after |
| landing (`simple-container.com`, `simple-forge.com`, `voice`) | 2026-10-13 | after |
| forge-runtime (`inference`) | 2026-10-11 | after |
| forge-talk (`talk`) | 2026-10-09 | after |
| forge-concierge (`concierge`, `meet`) | 2026-10-09 | after |
| forge-storage (`storage`) | 2026-10-07 | after |
| forge-notifier (`notifier`) | 2026-10-01 | after |
| forge-sessions (`sessions`) | 2026-10-01 | after |
| vpr-math-tutor (`vpr`) | **2026-09-27** | **BEFORE — the one outlier** |

(Source: `list_repositories` `updatedAt` plus `GET /commits?sha=main` per repo. `forge-notifier`'s
last successful Actions run is 2026-10-01T14:28:42Z, consistent with its commit date.)

**Read this correctly.** Repo activity is not a deploy, a deploy is not necessarily a registrar
re-provision, and none of this observes a worker script. It proves nothing about any domain. Its
only value is prioritisation: **13 of the 14 live in-scope domains sit well clear of the cut-over,
and `vpr.simple-forge.com` is the single domain whose owning repo has not been touched since
before `2026.9.16` was published.** If the operator wants to spend one Cloudflare API call instead
of sixteen, spend it on `vpr` first.

`vpr`'s last commit (2026-09-27) falls *between* the fix merge (09-23) and the release (09-30), so
even the screen cannot call it either way: it depends on whether that deploy's CI resolved `sc`
from `main` or from the latest release tag. That is a genuine unknown, not a guess I will dress up.

## What this audit does and does not settle

| Question | Settled? | By what |
|---|---|---|
| Is the fix in SC 2026.9.16? | **YES** | Finding 2 ancestry proof |
| What SHA should be cited? | **YES** | Finding 1 — `5252819589`, not `5d6d0afe` |
| Are `inference-test` / `forge-test` broken domains needing attention? | **YES — no** | Finding 3: they do not exist in DNS; nothing to fix for this slice |
| How many standing domains are in scope? | **YES — 14** | Finding 3 |
| Does any live domain carry an UNFIXED worker? | **NO** | Needs the Cloudflare token. Screen flags `vpr` as the only one not clearly past the cut-over. |

The slice therefore stays **`in-progress`**, blocked on the same single grant QA named — a
Cloudflare API token scoped to Workers-script read:

```
GET /client/v4/accounts/{account_id}/workers/scripts/{script_name}
# pass iff the body contains:  fwdHeaders.set("X-Forwarded-Host", origHost)
```

Fourteen calls closes this deterministically. One call against `vpr` closes the only domain the
provisioning evidence cannot vouch for.

## Ready-to-apply roadmap edit (cross-repo — `simple-container-com/forge`)

AC3's file is `docs/roadmap/items/sc-ingress-drops-visitor-host.md` in **`simple-container-com/forge`**,
not this repo. Not pushed from here, per the cross-repo guard. Apply verbatim:

Front matter — keep `status: in-progress`, and add:

```yaml
blocked_on: "Cloudflare API token scoped to Workers-script read (GET /client/v4/accounts/{id}/workers/scripts/{name})"
```

`## Status` row:

> Fleet audit of other domains · ⚠️ inconclusive 2026-10-14 · 14 standing in-scope domains (not 16 —
> `inference-test.simple-forge.com` is NXDOMAIN and `forge-test.simple-container.com` is NODATA on both
> Google and Cloudflare public resolvers, so neither is provisioned and neither has a worker to audit).
> All 14 live and Cloudflare-fronted. `X-Forwarded-Host` is unobservable black-box: its sole fleet
> consumer is auth-gated, writes to a dispatch payload rather than a response, and prefers
> `publicCallbackBaseURL`; QA's spoof-clobber discriminator is inconclusive by construction on
> single-tenant origins. Provisioning-side screen clears 13 of 14 domains past the SC 2026.9.16
> cut-over (published 2026-09-30); `vpr.simple-forge.com` is the only one whose owning repo
> (vpr-math-tutor, last commit 2026-09-27) predates it and cannot be called either way.
> **Correction:** the fix's SHA on `main` is `5252819589f8` (PR #406,
> 2026-09-23); the previously-cited `5d6d0afe` is a pre-merge commit not reachable from `main`.
> Evidence: `api` `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/` (`README.md` +
> `devops-provisioning-audit.md`).

`## History` — 2026-10-14 entry, same substance, plus: no domain requires a re-provision event on
account of this fix; the two absent `-test` domains are a separate "stack never deployed / destroyed"
question and should not be tracked under this slice.
