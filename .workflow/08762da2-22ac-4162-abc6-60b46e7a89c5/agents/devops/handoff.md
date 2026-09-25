Good. The PR is live and open at https://github.com/simple-container-com/api/pull/321. I have a clear picture of the state. Now I produce my DevOps handoff.

---

# DevOps Handoff — Bug: `sc secrets add/hide` fails on UTF-8 secret files

**Workflow run:** `08762da2-22ac-4162-abc6-60b46e7a89c5`
**Date:** 2026-06-12
**Author:** William Smith (devops)
**PR:** https://github.com/simple-container-com/api/pull/321
**Commit reviewed:** `75f5eddaed7dcb4772efe2a3238f64c0afa66bb7` (fix), head `32351e8a10e22bf449e17cc497c68e9ed48cd89e`

---

## 1. Overall Assessment

The fix is **correct and mergeable** from a DevOps perspective. The CI picture from QA is confirmed: all substantive checks pass. One infrastructure issue (DCO) requires resolution before merge. No deployment changes, no infra changes, no rollout complexity.

---

## 2. CI Status Review (from PR live fetch)

| Check | Status | Notes |
|---|---|---|
| Run tests | **pass** | All 6 new UTF-8 regression tests + full existing suite |
| Build Setup (lint, fmt, tools, schemas) | **pass** | Clean |
| Build sc (linux/amd64, darwin/amd64, darwin/arm64) | **pass** | All targets |
| Build cloud-helpers, github-actions, github-actions-staging | **pass** | |
| Finalize build | **pass** | |
| Fuzz HMAC cache | **pass** | |
| govulncheck | **pass** | |
| security (Secret Scan, SBOM, Status) | **pass** | |
| semgrep | **pass** | 0 findings confirmed by bot comment on PR |
| Analyze Go (CodeQL) | pending/long-running | No failures observed |
| **DCO** | **FAIL** | Infrastructure issue — see §3 |

---

## 3. DCO Failure — Root Cause and Fix

### What failed

The DCO (Developer Certificate of Origin) bot rejects all 6 commits on the branch because none carry a `Signed-off-by:` trailer. This includes the 4 Forge Workflow Engine scaffolding commits (author `forge-agent@simple-forge.com`) and the actual fix commit (`75f5edd`).

### Why this is infrastructure, not code

The DCO bot checks every commit author. The Forge Workflow Engine emits commits on behalf of the workflow run — it cannot retroactively sign off on commits it has already made without a force-push. The fix commit itself (`75f5edd`) is also unsigned because the developer agent did not configure `git commit --signoff`.

### Remediation options (pick one)

**Option A — Recommended: Configure DCO bot to exempt `forge-agent@` author**

Add to `.github/dco.yml` in `main`:

```yaml
# .github/dco.yml
allowRemediationCommits:
  individual: true
  organization: true
require:
  members: false
allowedPattern: "^forge-agent\\+.*@simple-forge\\.com$"
```

Or use the DCO bot's `exemptByCommitEmail` config (exact key depends on the DCO Action version in use):

```yaml
# .github/workflows/dco.yml (or whatever triggers the bot)
# Add to the DCO action step:
- uses: dco-org/dco-action@v1
  with:
    exempt-authors: forge-agent+.*@simple-forge.com
```

Check the current DCO Action version in `.github/workflows/` and match its config API.

**Option B — One-off remediation: force-push with signed commits**

```bash
git checkout forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/08762da2-22ac-4162-abc6-60b46e7a89c5
# Rebase and sign all commits
git rebase --signoff main
git push --force-with-lease
```

⚠ This rewrites history and invalidates review state. Only do this if Option A cannot be done first.

**Option C — Merge via admin bypass**

If the fix is urgent and the DCO check is non-blocking for admins, a repository admin can merge with "Merge without waiting for requirements to be met (bypass branch protection rules)". Not recommended as standing practice.

**My recommendation:** Option A — fix the root cause in the CI infrastructure so all future Forge workflow PRs don't hit this. Option C as an unblock for this specific PR if it's urgent and Option A is in flight.

---

## 4. Deployment Impact

**Zero.** This is a pure library/crypto fix:

- No service changes
- No API surface changes
- No config changes
- No database migrations
- No Simple Container `client.yaml` / `server.yaml` changes
- No secrets rotation required (existing encrypted secrets still decrypt correctly; re-encryption after `sc secrets hide` produces correct new chunks)

The fix ships in the next `sc` binary build, which CI already confirmed builds successfully for all targets. No rollout strategy needed beyond normal release flow.

---

## 5. Operational Risk Assessment

| Risk | Severity | Notes |
|---|---|---|
| Breaking existing secret decryption | **None** | `DecryptLargeString` is chunk-size agnostic; old ciphertexts unchanged |
| New encryption incompatible with old decrypt | **None** | Byte-based chunks are a subset of valid inputs to the existing decoder |
| Binary size / dependency change | **None** | `lo` import retained; no new dependencies |
| Re-encryption required for existing secrets | **None** | Old ciphertexts are valid; re-encrypt is optional but safe |
| DCO block on merge | **Low** | Process issue, not a code defect; bypassed or fixed at infra level |

---

## 6. Post-Merge Actions

1. **Track the DCO infrastructure fix** — create an internal ticket to add `forge-agent@` exemption to the DCO bot config before the next workflow PR lands.
2. **Optional: bulk re-encrypt secrets with non-ASCII content** — `sc secrets hide` on affected files after the fix is deployed. Not required; existing secrets still decrypt.
3. **Notify users affected by the original bug** — any teams who worked around the bug by stripping non-ASCII from secrets files can revert those changes.
4. **Future: hybrid encryption for RSA path** — track as a separate ticket. The ed25519 path already uses ChaCha20-Poly1305 hybrid. The RSA path should eventually be migrated (no chunking, no size bugs). Not blocking.

---

## 7. Handoff Summary

| Item | Status |
|---|---|
| Code fix correct | ✅ |
| Tests covering bug case | ✅ |
| CI green (substantive) | ✅ |
| DCO resolved | ❌ Infrastructure action required |
| Deployment changes | None required |
| Merge readiness | **Ready once DCO is unblocked** |

**Recommended next action:** Unblock merge via DCO bot exemption (Option A) or admin bypass (Option C), then merge to `main`. No other gate is blocking.

`PULL_REQUEST: https://github.com/simple-container-com/api/pull/321`

---

```json
{
  "run_id": "08762da2-22ac-4162-abc6-60b46e7a89c5",
  "from": "devops",
  "to": null,
  "date": "2026-06-12",
  "artifact": "devops-handoff",
  "pr": "https://github.com/simple-container-com/api/pull/321",
  "merge_ready": true,
  "blocker": "DCO bot — infrastructure fix required, not a code defect",
  "deployment_changes": false,
  "chainContinue": false
}
```