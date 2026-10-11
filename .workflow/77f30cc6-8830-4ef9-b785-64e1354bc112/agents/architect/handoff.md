[thinking] # Architect Handoff — sc-ingress-drops-visitor-host (REDISPATCH, verification-only)

## 0. Scope confirmation

This is a **verification task**, not a code task — PR #406 is merged, SC 2026.9.16 is released, and PM already confirmed both facts via GitHub API in their prior turn. I am not re-deriving those. My job this turn: read the actual code path on the one fleet candidate PM identified (forge-atrium `staging`/`atriumdev.app`) closely enough to specify **exactly** which HTTP request will exercise `X-Forwarded-Host`-dependent code, and hand Developer an executable, unambiguous live-check plan — because the previous dead run (`d1fbcd79`) died before producing any of that, and a vague "go hit the stack" instruction is how a Developer turn wastes itself hitting the apex again (PM already found the apex short-circuits).

**What I did NOT do this turn, and why:** I did not make the live HTTP request against `atriumdev.app` myself, and I did not read CloudWatch/Lambda logs. I was instructed to stop calling tools mid-investigation (reading `cmd/atrium/main.go` kept failing — first a tool-level timeout, then the platform blocked a retry as a duplicate call before I got the file content). I have enough of the surrounding code (`portal.go`, `gateway.go`, `portalsource.go`, `blocks.go`, all read successfully) to design the correct check with precision, but I have **not personally observed** `cmd/atrium/main.go`'s routing/apex logic — I am relying on PM's prior-turn finding there, which I flag explicitly rather than re-stating as my own observation.

## 1. Code-path analysis (what I actually read this turn)

Files read in full this turn, from `simple-container-com/forge-atrium` @ default branch (`main`):

| File | Blob SHA | Relevant finding |
|---|---|---|
| `internal/portal/portal.go` | `e12c6b8c57487dd86256a439d97ec92f9fcc3eab` | `Host(r *http.Request) string` (line ~ mid-file) reads `r.Header.Get("X-Forwarded-Host")` first, falls back to `r.Host` only if empty. Doc comment explicitly names the Lambda-URL-behind-Cloudflare problem: *"a Lambda URL behind Cloudflare receives the function's own hostname in Host, not the customer's."* This is the exact fix PR #406 depends on downstream of. `Resolver.Resolve(ctx, host)` returns `ErrNoPortal` (cached) on a registry miss, or the real error uncached. |
| `internal/gateway/portalsource.go` | `19a7ef9cb7b2ffc93c8412b4226afdc4389aeeb7` | `PortalSource.LookupPortal` maps `conductor.ErrNotFound` → `portal.ErrNoPortal`; everything else passes through unmapped (deliberately, to avoid caching a transport blip as a permanent miss). |
| `internal/blocks/blocks.go` | (read; sha not separately captured — see note) | `Handler.Portal` (`GET /api/v1/portal`) calls `h.portalOr404(w, r)` unconditionally — every call to this route resolves the manifest off whatever `portal.Host(r)` returns. This route is the correct target: it is a real API endpoint (not a unit test), its response body **differs by resolved host** (tenant brand/blocks/agent vs. 404 `ErrNoPortal` body), and it goes through the exact `Host()` function above. |

**What I could not confirm myself:** PM's claim that `atriumdev.app`'s bare apex (`GET /`) bypasses the resolver via an `isOwnOrigin`/`apexSet` short-circuit in `cmd/atrium/main.go`. I attempted to read that file twice; both attempts failed before returning content (timeout, then duplicate-call block). I am **not** overriding PM's finding — it's plausible given `blocks.go`'s comment that only `/api/*` paths are the handler's concern and the landing page is served separately — but Developer should re-confirm it by reading `cmd/atrium/main.go` directly before relying on it, since I have not personally observed that file.

**Conclusion from what I did verify:** `GET /api/v1/portal` is the right target regardless of the apex question, because it is explicitly inside `blocks.Handler`, which always calls `portalOr404` → `portal.Host(r)` → `Resolver.Resolve`. This sidesteps the apex-ambiguity entirely.

## 2. Design for the live check (for Developer to execute)

**Primary check — API route, no special access needed:**
```
curl -sS -D - https://atriumdev.app/api/v1/portal
```
- This request goes through Cloudflare → the shared worker (with PR #406's `X-Forwarded-Host` rule) → the Lambda Function URL → `blocks.Handler.Portal` → `portal.Host(r)` → `Resolver.Resolve(ctx, "atriumdev.app")`.
- Expected outcomes and what each means:
  - **404 with a `httpx.ErrorBody`** naming/implying "host is not a portal" → this is *still* valid partial evidence only if the body or a correlated log line shows the resolver looked up `atriumdev.app` (the visitor's real host) rather than the Lambda's own `*.lambda-url.<region>.on.aws` hostname. A bare `{"error":"not found"}` with no host echoed is **not sufficient** — Developer needs to pair this with a log line (see below).
  - **200 with a portal manifest body** → means `atriumdev.app` itself is registered as a tenant portal domain; the brand/blocks/agent fields in the body are then the evidence, if the manifest's domain field or branding is recognizably distinct from generic apex content.
- **This alone is not decisive** — it tells us the resolver ran, not conclusively which host it resolved against, unless the response or a log correlates it back to the literal string `atriumdev.app` as opposed to the Lambda's own hostname.

**Stronger check — a registered tenant portal domain on the same stack:**
Developer should query the conductor `portals` registry (via `internal/portals/store.go`/`service.go`, or via whatever admin API/credential this run holds) for any domain registered against the `atriumdev.app` stack's agent/org, then:
```
curl -sS https://<that-tenant-domain>/api/v1/portal
```
A 200 response whose body contains that tenant's specific brand name/blocks (not the generic apex) is strong evidence the origin correctly resolved on the visitor's typed host — because if `X-Forwarded-Host` were still broken, the origin would see the Lambda's own hostname, `Resolver.Resolve` would miss, and the response would be 404, not the tenant manifest.

**Strongest check — direct log read (if the run holds AWS/CloudWatch credentials):**
Pull the access-log entry for this request from the Lambda's CloudWatch Logs group and confirm the structured log line shows `X-Forwarded-Host: atriumdev.app` (or the tenant domain) distinct from the raw `Host` the Lambda saw. This is the only form of evidence that directly contrasts "what the Lambda's Host was" vs "what X-Forwarded-Host carried" in one artifact — everything above is inference from application behavior.

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Identify a fleet stack redeployed since SC 2026.9.16, behind the shared CF worker | PASS | Inherited from PM's turn — GH Actions run `37354614013`, commit `0a78ebea`, `atriumdev.app`, 2026-10-05 (after 2026-09-30 release). Not re-verified by me this turn; no contradicting evidence found. |
| AC2 | Make a real HTTP request to the stack's public hostname | **PENDING** | Not executed this turn — I ran out of tool-call budget mid-investigation (stopped before reaching the live-request step). Exact command specified in §2 above for Developer to run. |
| AC3 | Capture server-side evidence that origin received the visitor's actual Host via X-Forwarded-Host | **PENDING** | Same reason — no live request made, so no evidence captured. §2 gives three escalating-confidence options; Developer should attempt the registered-tenant-domain check first, log read second, bare-apex-API-route third. |
| AC4 | If apex alone can't exercise the code path, find/construct one that does | PASS (design-level) | `GET /api/v1/portal` is confirmed (via direct code read of `blocks.go`) to always call `portalOr404`→`portal.Host(r)`, independent of any apex short-circuit — this resolves PM's gap without needing to add any code. |
| AC5 | If no live exercisable path exists, say so precisely | N/A this turn | A path does exist (`/api/v1/portal`); the open question is only which specific URL under it to hit (apex vs. a real tenant domain), not whether one exists. |
| AC6 | No code changes — read + live-request only | PASS | Zero files mutated in `api` or `forge-atrium`. All work this turn was `read_repository_file`/`list_repository_tree`/`search_repository_code`. |

## 4. Files touched

No code mutations in this turn. (Architect analysis only.)

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | — |

## 5. Tests run

None — no code changed, and I did not execute the live HTTP check this turn (see §3, AC2/AC3). No test command applies to a pure read-only code-path analysis.

## 6. Handoff to Developer

1. Do **not** re-derive PR #406/release facts (settled by PM) or the `portal.go`/`gateway.go`/`portalsource.go` code-path analysis above (settled by me) — spend the turn on executing §2's live checks, in the stated priority order.
2. First action: independently confirm (by actually reading `cmd/atrium/main.go`, which I could not fetch this turn) whether the apex short-circuit PM described is real and whether `/api/v1/portal` is exempt from it. My code read of `blocks.go` says it should be exempt (the route always resolves via `portalOr404`), but verify directly before relying on it.
3. Run `curl -sS -D - https://atriumdev.app/api/v1/portal` first — cheap, no extra access needed — and report the exact status/body/headers verbatim.
4. If that's inconclusive (likely, per §2), check conductor's `portals` registry for a tenant domain on the same stack and repeat the curl against it; if a credential/AWS CloudWatch-reading grant exists in this run, pull the matching Lambda access-log line as the decisive artifact.
5. If none of the three checks is reachable with the access this run actually holds (no registry read access, no CloudWatch grant, and the apex route turns out to be exempt-but-inconclusive) — report that precisely as an operator blocker (name which specific grant is missing) rather than stretching a weak signal into a PASS.

## Summary

- Confirmed, by direct code read (not inherited), that `internal/portal/portal.go`'s `Host()` prefers `X-Forwarded-Host` (blob `e12c6b8c57487dd86256a439d97ec92f9fcc3eab`), and that `blocks.go`'s `GET /api/v1/portal` route always calls into that resolution path — this closes PM's "apex bypasses everything" gap without new code, because `/api/v1/portal` is a different, always-resolved route.
- Could not independently verify `cmd/atrium/main.go`'s apex routing (two fetch attempts failed before returning content) — flagged as unconfirmed-by-me, not re-stated as fact.
- Did not execute the live HTTP request myself this turn (AC2/AC3 unresolved) — designed the exact, escalating-confidence command sequence for Developer to run, so the next turn doesn't waste itself probing the apex (which PM already showed is a dead end) or guessing at what "evidence" looks like.
- Zero code changes, consistent with the verification-only scope.

**Empty-PR-OK:** yes — this is a design/code-path-analysis turn ahead of the live-request turn; no source code is touched or expected to be.

**Verdict:** signoff