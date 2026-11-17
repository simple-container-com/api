Fix the `Unrecognized named-value: 'env'` break in simple-container-com/api's shared GitHub Actions. Commit efbb76d079 ("ci: bake RU Trusted Root CA into runner image + forward SSL_CERT_FILE", #624) added `SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}` to the `runs.env:` block of FOUR composite/docker actions: .github/actions/deploy-client-stack/action.yml (line 77), .github/actions/destroy/action.yml (line 80), .github/actions/cancel-stack/action.yml (line 79), and .github/actions/provision-parent-stack/action.yml. The `env` context does NOT exist when GitHub parses an action's metadata file, so every consumer repo fails instantly at the "Set up job" step, before any step executes, with: "Failed to load simple-container-com/api/main/.github/actions/deploy-client-stack/action.yml" + "(Line: 77, Col: 20): Unrecognized named-value: 'env'. Located at position 1 within expression: env.SSL_CERT_FILE". Verified live on forge-conductor deploy runs 43749428441 and 43785139401 (both completed/failure at "Set up job"). This is a FLEET-WIDE CI outage: api@main is still broken as of 2026-11-16T01:20Z, and every simple-container-com service that deploys via `@main` (forge-conductor, forge-aigateway, forge-sessions, forge-notifier, forge-storage, forge-runtime, landing, vpr-math-tutor) will fail its next deploy — conductor was simply the first to attempt one after the break. Correct fix: a docker action cannot read the caller's workflow `env` from its metadata. Either (a) declare an explicit `ssl-cert-file` INPUT on each action and have callers pass it in `with:` (inputs ARE valid in runs.env), or (b) set SSL_CERT_FILE to the known baked-in path inside the image entrypoint, since efbb76d079 already bakes the CA bundle into github-actions.Dockerfile (+21 lines) — the env forwarding is redundant with the image change for callers that just need the bundle present. Note the CA bundle itself is in the IMAGE, so consumers pinning to main~1 (2fce01f64a8cafef8b24fe755a8982229280c237) still get the CA fix and only lose the env forwarding. forge-conductor has pinned to that full SHA as a local workaround (commit d90757f7c3) and must be reverted to @main once api is fixed.

── Source from the workflow that triggered this (context only — it is from a DIFFERENT run, often a different repo; do NOT reuse its commit SHA, branch, or file paths as your own) ──
All gates executed and state cleaned up. Here is my handoff.

---

# QA Handoff — `preview-workflow-filename-mismatch` (3rd dispatch, live-verify)

**Verdict:** failure

AC1 — this slice's actual panic fix — **passes live on staging, proven by direct probe**. AC5 could not be live-verified, because installing the preview workflow on any fixture repo is blocked by a **fourth, newly-discovered defect** in conductor's own setup endpoint. I also **disproved** the prior run's "org-scoping / GitHub-App-installation" theory — the real cause was benign and is now self-reporting.

## 1. Commit + branch identity

No code mutations in this turn (QA verification role; working tree clean — `git status --short` empty).

Branch verified as carrying real work, not just engine metadata:

```
$ git log -3 --format="%H %s"
«sha-from-triggering-run-redacted» forge: character developer (David Black) phase run idempotency: developer:run
«sha-from-triggering-run-redacted» ci: use the full 40-char SHA for the pinned api action refs
«sha-from-triggering-run-redacted» ci: pin simple-container-com/api actions to main~1 to clear 'env' metadata break
```

Substance gate: `git diff origin/main...HEAD --stat` shows **2 real source rows** (`.github/workflows/deploy-forge-conductor.yml` +21, `destroy-forge-conductor.yml` +4) alongside 12 `.workflow/` rows. Not a no-op PR.

**I verified the developer's load-bearing claim rather than taking it:** `git diff origin/main...HEAD -- . ':(exclude).workflow' ':(exclude).github/workflows'` → **empty**. The branch's Go source is byte-identical to `origin/main` (`bd008541e9` = PR #676 merge), so the green deploy genuinely shipped `main`'s code.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files mutated (verification-only turn) |

Remote state I mutated and reverted: `previewLinkTemplate` on `worm-team/forge-pipeline-mvp` (repo id `6a0c2982…`) — set as an AC5 precondition, reverted to `''` (confirmed `HTTP=200`, `previewLinkTemplate now = ''`).

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC-pre | Green Deploy run on conductor code ≥ `bd008541e9` | **PASS** | `gh run view 43786715794` → `{"conclusion":"success","headSha":"d90757f7c3…","updatedAt":"2026-11-16T01:41:14Z"}`. Prior runs `43749428441` + `43785139401` both `failure` on `main`/`bd008541e9`. Branch source ≡ `main` (§1), so staging runs both fixes. [Run URL](https://github.com/simple-container-com/forge-conductor/actions/runs/43786715794) |
| AC1 | `POST /api/repositories/setup` responds without panicking | **PASS** | Live, 4 repos, authenticated: `HTTP=403 {"success":false,"message":"Permission check failed","errors":[{"code":"PERMISSION_DENIED","message":"Push access is required to set up workflows"}]}`. **Why this proves the fix:** that string exists only at `setup.go:238` inside `checkPermissions`, which **dereferences `githubClient`** at `setup.go:221` — the exact pre-fix panic site. Reaching it means `resolveClient` (`setup.go:93`) returned a non-nil client and a live GitHub API call succeeded. Pre-fix this nil-deref'd on every well-formed request. Structured JSON, no 500, no panic trace. |
| AC5 | `GET /api/issues/:issueId/preview-links` returns real status, not `unknown` + "Failed to get workflow runs" | **FAIL — blocked (new distinct defect, see §4)** | Live: `HTTP=200`, `status:"unknown"`, `errorMsg:"Preview workflow \"preview-env.yaml\" is not installed in worm-team/forge-pipeline-mvp (no .github/workflows/preview-env.yaml on branch forge/pipeline/81/e2e-smoke-team)"`. Brief's literal pass condition (`status` ∈ success/failure/pending) **not met**. Half the regression *is* fixed: the message is no longer "Failed to get workflow runs". Cannot complete — installing the workflow requires the setup endpoint, which 403s (§4). |
| AC6 | Restatement of AC5 | **FAIL — blocked** | As AC5. |
| Distinct-finding | Prior "org-scoping 404" named distinctly, not conflated | **PASS — and disproved** | See §4. The 404 was **not** an App-installation defect; the fixture repo has no workflows installed at all. `previewlinks/service.go:139-156` now says so explicitly. |
| Cleanup | Fixture debris removed from `refapp` | **PASS (nothing created)** | `refapp` `.github/workflows` → 404; open PRs → `0`. Setup 403'd, so nothing was ever installed. `previewLinkTemplate` mutation reverted. |
| Lint | Static-lint gate | **N/A (no-op)** | No `.golangci.yml`/`.golangci.yaml`, no `make lint` target. Ran `go vet` on both packages as proxy → **exit 0**. |

I am **not** marking AC5 PASS. The error text improved materially, but the brief's pass condition is a real status, and I could not produce one.

## 4. The blocker — a fourth distinct defect (conductor's own)

`POST /api/repositories/setup` is **unreachable for API-key callers**: identical `403 PERMISSION_DENIED` on `refapp`, `backup-smoke-tier4`, `forge-share`, `two-peaks` — including repos that demonstrably have workflow files installed. Not repo-specific.

Root cause is **already documented in the code, with no guard** (`setup.go:212-219`):

> `perms.Push` is always false for a GitHub App installation token … so routing an App-authenticated setup through here would reject it with `ErrCodePermissionDenied` despite the installation having write. Today every caller is user-authenticated, which is why this has not bitten.

The admin-Bearer path routes through the App installation, so that assumption is false. Fix per the code's own note: check write via `GET /installation/repositories` or attempt-the-write, not `perms.Push`. **`record_task` was refused (`max follow-up fan-out (2) reached for this run`) — this needs manual filing as a roadmap item against forge-conductor.**

**Correcting the prior run's diagnosis:** the earlier dispatch concluded conductor "could not list workflow runs under ANY workflow name" on `worm-team/for
…(truncated)
workflowTriggerParentRunId: df26d77a-e5ce-4fc8-9076-6cc4146be60f
workflowTriggerRootRunId: 7867e845-ad54-4a2a-a7c6-d918c2ca4cc0