Apply the sc-ingress-drops-visitor-host audit result to the roadmap item in simple-container-com/forge (this is a CROSS-REPO edit the audit run could not make — its linked repo was simple-container-com/api). Edit docs/roadmap/items/sc-ingress-drops-visitor-host.md: keep `status: in-progress`; add `blocked_on: "Cloudflare API token scoped to Workers-script read (GET /client/v4/accounts/{id}/workers/scripts/{name})"`; add the `## Status` row and `## History` entry whose exact verbatim text is already written out in the "Ready-to-apply roadmap edit" section of simple-container-com/api `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/devops-provisioning-audit.md` (commit 3f32326c6c). Three corrections MUST carry into the item: (1) the fix's SHA on main is 5252819589f8c11f4a4a450c1dbcc4602e1913cb, NOT 5d6d0afe (which is a pre-merge commit unreachable from main — merge-base proof in the doc); (2) inference-test.simple-forge.com is NXDOMAIN and forge-test.simple-container.com is NODATA on both Google and Cloudflare public resolvers, so they are NOT provisioned, have no worker to audit, and must NOT be tracked as broken under this slice — standing in-scope domain count is 14, not 16; (3) no domain needs a re-provision event on account of this fix. Do NOT mark the slice shipped: 14 domains remain UNVERIFIABLE black-box and closing them needs an operator-granted Cloudflare Workers-script read token. If that token is granted, audit vpr.simple-forge.com FIRST — it is the only domain whose owning repo (vpr-math-tutor, last commit 2026-09-27) predates the SC 2026.9.16 release cut-over of 2026-09-30.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
# DevOps Handoff — sc-ingress-drops-visitor-host: provisioning-side audit, three findings (two corrections)

## 0. What I did differently, and why

QA correctly ruled the black-box audit **inconclusive by construction** and told the next turn not to re-probe the 15 domains hoping for a different answer. I didn't. Re-running an inconclusive probe produces nothing.

Instead I audited the **provisioning side** — credential-free, and it answers a question no amount of HTTP probing can: *which domains are even eligible to carry the fixed worker, and which aren't provisioned at all.* That produced three findings, two of which correct the record this chain had built, including the brief's own.

## 1. Commit + branch identity

- **Commits:** `3f32326c6c` (new DevOps audit doc), `30cc539aee` (cross-link + corrections into the README)
- **Branch:** `«branch-from-triggering-run-redacted»`
- Verified real, not described: `list_repository_commits` on `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/` → `3f32326c6c` (17:53:05Z) and on `README.md` → `30cc539aee` (17:54:22Z), both `simple-container-forge[bot]`, this run.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/devops-provisioning-audit.md` | +218/-0 (11,567 B, new) | Findings 1–3 with verbatim API/resolver output; deploy-window screen; settled-vs-unsettled table; ready-to-apply cross-repo roadmap text |
| `docs/implementation/2026-10-14/sc-ingress-visitor-host-audit/README.md` | +36/-14 (14,308 B) | Correction banner at top; rows 7/13 → **N/A — not provisioned**; `vpr` flagged as priority; QA §5 marked superseded with credit; bottom line → 14 domains |

No `api` source touched — both paths are under `docs/`.

## 3. The three findings

**Finding 1 — the fix's SHA is `5252819589`, not `5d6d0afe`.** The brief, the roadmap item, and all four prior handoffs cite `5d6d0afe`. That object exists but is **not reachable from `main`**: `compare/2026.9.16...5d6d0afe` returns `merge_base = eeaba3c0`, which is `5d6d0afe`'s own *parent* — the signature of a pre-squash commit. The real one is `«sha-from-triggering-run-redacted»` ("fix(cloudflare): … (#406)", PGP verified), and `compare/5252819589...2026.9.16` returns `merge_base = 5252819589` itself — that equality *is* the ancestry proof. Same tree (`96c2740845`) on both, so the fix's content was never in doubt; the citation was. It matters because `5d6d0afe` is the SHA an operator would bisect with, and it isn't on the release line.

**Finding 2 — SC `2026.9.16` really does contain the fix.** Published 2026-09-30T10:54:52Z, tag commit `«sha-from-triggering-run-redacted»`. Combined with Finding 1: fix merged 09-23, released 09-30. The brief's release claim is correct as stated. Cut-over = **2026-09-30** (release-pinned CI) or **2026-09-23** (CI on `main`).

**Finding 3 (material) — the two "dead" domains have no DNS record at all.** Both prior turns recorded `198.18.140.62/.63`, TLS completing then HTTP dying, and reasoned toward "DNS outliving a torn-down stack". Checked against two public resolvers from outside the sandbox:

```
dns.google  inference-test.simple-forge.com   A     → Status 3 (NXDOMAIN)
cloudflare  inference-test.simple-forge.com   A     → Status 3 (NXDOMAIN)
dns.google  forge-test.simple-container.com   A/AAAA/CNAME/TXT → Status 0, NO Answer (NODATA + SOA)
cloudflare  forge-test.simple-container.com   A     → Status 0, NO Answer
# live siblings, for contrast:
inference.simple-forge.com → 172.67.165.92, 104.21.91.30
forge.simple-container.com → 104.21.34.207, 172.67.208.99
```

`198.18.0.0/15` is RFC 2544 benchmark space — the sandbox DNS-NAT **synthesises** an address there for a name it can't resolve, which is also why TLS "completed" (against the NAT) and HTTP then died. QA's instinct that these addresses were NAT-adjacent was right; the public check confirms the names don't exist.

This changes the verdict, not just the explanation: **no DNS record → no proxied CNAME → no minted worker.** There is no artifact to audit. Correct verdict is **N/A — not provisioned**, and the AC4 answer becomes *"no domain needs a re-provision event because of this fix; two domains do not exist"* — not *"two domains are broken"*. **Standing in-scope count is 14, not 16.**

**Deploy-window screen (a risk bound, explicitly NOT a pass).** PR #406's own blast-radius note: *"every stack must redeploy to pick up the new script."* Repo activity is a necessary-condition proxy in a deploy-on-push fleet: 13 of the 14 live domains' owning repos were touched 2026-10-01 … 10-14, comfortably past the cut-over. **`vpr.simple-forge.com` is the sole outlier** — `vpr-math-tutor`'s last commit is 2026-09-27, which falls *between* merge (09-23) and release (09-30), so the screen can't call it either way. Repo activity is not a deploy and a deploy isn't necessarily a registrar re-provision; this proves nothing per-domain. Its only value is prioritisation: **if the operator spends one Cloudflare call instead of fourteen, spend it on `vpr`.**

## 4. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Live HTTP request per in-scope domain, recorded verbatim | **PASS** (inherited + corrected) | Developer + QA swept all 16 independently; table in `README.md`. I corrected rows 7 and 13 from `UNREACHABLE` to `N/A — not provisioned` on resolver evidence rather than re-probing. |
| AC2 | Negative/control case per domain that would fail if host routing weren't working | **FAIL-blocked** | Unachievable black-box on all 14 live domains. Sole XFH consumer is auth-gated and writes to a dispatch payload, not a response; QA's spoof-clobber discriminator is inconclusive by construction on single-tenant origins. Needs **Cloudflare API token scoped to Workers-script read**. Not held by any character in this chain. |
| AC3 | Tab
…(truncated)
workflowTriggerParentRunId: 5e639f8d-27fd-44f2-8ea4-a1e7e7d49fff
workflowTriggerRootRunId: 74ed7da4-6ff1-4198-8ced-5b434e4ab271