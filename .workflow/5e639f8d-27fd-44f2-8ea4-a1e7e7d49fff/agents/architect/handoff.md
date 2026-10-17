[thinking] # Architect Handoff — sc-ingress-drops-visitor-host: Design confirmation + audit execution plan

## 0. What I verified this turn (real tool calls only)

I read the actual provisioning code directly from `simple-container-com/api` (not relying on PM's paraphrase):

```
read_repository_file: pkg/clouds/pulumi/cloudflare/registrar.go
sha: c9929918b512
```

Confirmed, verbatim from the file:
- `ProvisionDomainForEndpoint` creates a proxied CNAME (`r.NewRecord`) **then** calls `r.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{Name: endpoint.Name, FromHost: endpoint.Domain, ToHost: endpoint.TargetHost})` — i.e. every stack provisioned through this function gets a worker minted.
- Inside `NewOverrideHeaderRule`, the generated `workerScriptCode` template contains exactly:
  ```js
  const overrideHost = "%s";
  const url = new URL(origRequest.url);
  const origHost = new URL(origRequest.url).hostname;
  url.hostname = overrideHost;
  const fwdHeaders = new Headers(origRequest.headers);
  fwdHeaders.set("X-Forwarded-Host", origHost);
  ```
  This is the fixed behavior (PR #406) — `origHost` (visitor's real hostname) is captured before `url.hostname` is overwritten, then forwarded via `X-Forwarded-Host` with `set()` (not `append()`, so a spoofed client header is clobbered). This matches the brief's description precisely and confirms PM's §4 claim that `NewOverrideHeaderRule` is the single code path the fix touched.
- `overrideHost`/`ToHost` is baked in as a **per-worker string literal** substituted via `sdk.Sprintf(workerScriptCode, ..., rule.ToHost, ...)` at provision time — confirming the brief's root finding: there is no shared worker, each domain gets its own minted script, so `atriumdev.app`'s pass says nothing about any other domain's script.

**I did not re-fetch `yandex/registrar.go` or `aws/ecs_fargate.go` this turn** (budget/tool-call constraint) — I'm relying on PM's citation of those two files for the ECS-Fargate/YC exclusion. That exclusion is architecturally sound on its face (different provisioning function = different code, can't be implicated by a Worker-script bug) but I flag it as **PM-sourced, not independently re-verified by me this turn** — Developer/QA should treat it as reliable but not re-derive it from scratch each time.

**Verdict on PM's scope split (§4 of PM handoff): CONFIRMED** for the Cloudflare-Worker mechanism. The 16-domain in-scope list stands as the audit target.

## 1. Commit + branch identity

No code mutations in this turn — Architect role, design/planning only. (Per the task's `api`-repo constraint, no source change was expected or attempted; branch-protection on `simple-container-com/api` blocks bot auto-merge regardless.)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | none this turn |

## 3. Open finding — roadmap item file location could not be confirmed this turn

I attempted to locate `docs/roadmap/items/sc-ingress-drops-visitor-host.md` (the file the brief's Definition of Done requires editing) in `simple-container-com/api` to check its current front-matter/History before planning the write. Real tool outputs:

```
read_repository_file(api, docs/roadmap/items/sc-ingress-drops-visitor-host.md)
→ error: file simple-container-com/api/docs/roadmap/items/sc-ingress-drops-visitor-host.md not found at ref ""

list_repository_tree(api, path="docs/roadmap")        → {"entries":[]}
list_repository_tree(api, path="docs/roadmap/items")  → {"entries":[]}
list_repository_tree(api, path="docs")                → {"entries":[{"path":"docs","type":"tree"}]}  (anomalous self-referential nesting — unresolved)
search_repository_code(query="sc-ingress-drops-visitor-host") → 0 hits in api/forge/etc.; only hits were in forge-conductor's own test fixtures referencing this slice-id (dev_signoff_verify.go, verification_no_diff_test.go), not the roadmap doc itself
list_repository_tree(forge, path="docs/roadmap")        → {"entries":[]}
list_repository_tree(forge, path="docs/roadmap/items")  → {"entries":[]}
list_repository_tree(forge-conductor, path="docs/roadmap") → {"entries":[]}
```

**This is a genuine, reproducible gap, not a guess**: the roadmap item file the brief asks us to edit does not resolve at the expected path in `api`, `forge`, or `forge-conductor` on their default branches via the repo-read tools available to me this turn. PM's handoff claimed to have "read" this file's full History, but I could not reproduce that read. Two honest possibilities, neither of which I can resolve further without more tool calls:
1. The roadmap tree lives at a path I haven't guessed (e.g. nested `docs/docs/roadmap/...` given the anomalous listing above), or in a repo not in my onboarded list.
2. PM's read happened against a different ref/branch context (e.g. the triggering root run's own workflow-storage branch) that isn't the `api` default branch, and got conflated with "the repo."

**Action for downstream (Developer/QA/DevOps):** before editing `items/sc-ingress-drops-visitor-host.md`, run `list_repository_tree` with `recursive=true` scoped narrowly (e.g. `path="docs"` on each repo) to find the real path, or `git log --all --full-history -- '**/sc-ingress-drops-visitor-host.md'` from a clone. Do NOT assume the PM's cited path/content is accurate without re-confirming — treat it as **PROVISIONAL** per this run's anti-fabrication policy. I am not fabricating a path or front-matter content I haven't observed.

## 4. Audit execution plan (for Developer/QA — concrete, not hand-wavy)

For each of PM's 16 in-scope domains, run the **direct-vs-proxied diff**, the literal transplant of the `atriumdev.app` proof:

**Positive case (per domain):**
```
curl -s -D - -o /dev/null https://<domain>/<known-good-path>
```
Expect 200 (or whatever the service's healthy status is) — this proves the Cloudflare-fronted path still works end-to-end post-fix.

**Host-forwarding evidence (the actual AC2 control) — two interchangeable methods, pick per service:**
- **A. Echo/debug endpoint** (preferred where available): hit a path that reflects request headers back (many `go-aws-lambda-sdk`-based services expose `/health` or similar — check each service's handler registration before assuming). A pass is the response body/header showing `X-Forwarded-Host: <domain>` where `<domain>` is the real visitor hostname, not the Lambda's internal Function URL hostname.
- **B. CloudWatch Logs read** (fallback, no code change needed): `aws logs filter-log-events` on the Lambda's log group for a recent invocation, grep for the `X-Forwarded-Host` header value logged by the Lambda's request-logging middleware (if present) — confirms what reached the origin, independent of what the service does with it.

**Negative/control case:** request the **origin directly**, bypassing Cloudflare — either the raw Lambda Function URL (visible in Pulumi stack export / AWS Lambda console under "Function URL") or the S3 bucket's origin domain for static sites. This request will either 403 (no public Function URL exposed) or return content keyed to the Lambda's own `overrideHost`, not the visitor's domain — demonstrating that the Cloudflare Worker is the thing doing the host-rewrite work, not some other layer. **Do not fabricate this response** — if the Function URL is not resolvable without a grant this run lacks, say so explicitly per domain rather than guessing the result.

**Table shape to populate (16 rows, example below) — commit this into the roadmap item (location per §3 above), not just the handoff:**

| # | Domain | Positive check | X-Fwd-Host evidence | Control (direct origin) | Pass/Fail |
|---|---|---|---|---|---|
| 1 | app.simple-forge.com | `curl -D- https://app.simple-forge.com/<path>` → `<status>` | `<header/log value observed>` | `<direct Lambda URL result>` | PASS/FAIL |
| … | … | … | … | … | … |

I am not populating this table myself — I made zero live HTTP requests this turn (that is explicitly Developer/QA's deliverable per the brief, and I was instructed to stop calling tools before executing it).

## 5. Acceptance criteria verification (brief's AC1–AC6, from PM §6)

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Live HTTP request per in-scope domain, result recorded | FAIL-blocked | Not executed this turn — architecture/design turn only. Deferred to Developer/QA with the exact command shapes in §4. |
| AC2 | Each domain has a negative/control case | FAIL-blocked | Control methodology specified in §4 (direct-origin bypass); not executed. |
| AC3 | Pass/fail table committed into `items/sc-ingress-drops-visitor-host.md` | FAIL-blocked | File path unresolved this turn — see §3. Must be located before any edit can land. |
| AC4 | Name precisely any domain needing re-provision | N/A this turn | Depends on AC1/AC2 results not yet gathered. |
| AC5 | Flip `status: shipped` if all pass | N/A this turn | Depends on AC1/AC2/AC3. |
| AC6 | No `api` source file touched | PASS | Confirmed — I read `pkg/clouds/pulumi/cloudflare/registrar.go` but wrote nothing to it; no source diff exists in this turn. |

## 6. Tests run

None — no code was mutated, no test suite applies to a design-only turn. The verification I performed was reading one source file via `read_repository_file` (output quoted verbatim in §0) and six `list_repository_tree`/`search_repository_code` probes for the roadmap-doc location (output quoted verbatim in §3).

## 7. Notes for Developer/QA

- PM's 16-domain scope (§4 of their handoff) is architecturally confirmed for the Cloudflare-Worker mechanism — use it as-is.
- Execute §4's test plan per domain. If you lack AWS console/CloudWatch access or a Cloudflare API token to read the minted script's `overrideHost` literal, **stop at that domain and report the missing grant by name** — do not guess a pass.
- Resolve the roadmap-item file location (§3) before attempting the required doc edit; if it genuinely doesn't exist at any discoverable path, create it fresh under `docs/roadmap/items/sc-ingress-drops-visitor-host.md` in `api` with the full audit table, rather than silently writing nowhere.
- `slice_id: sc-ingress-drops-visitor-host` for downstream resolution.

**Empty-PR-OK:** yes — this is an architecture/design-confirmation turn with no source mutation expected; the brief's live-audit deliverable is explicitly QA/Developer's, not Architect's.

**Verdict:** signoff
**Provisional:** yes — the roadmap item file's path could not be confirmed in this repo this turn (see §3); downstream must locate/clone-and-verify the real path, and treat PM's cited file content as a hypothesis, not an observed fact, until re-confirmed.