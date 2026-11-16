# QA Handoff — fix `Unrecognized named-value: 'env'` in api's shared GitHub Actions (verification turn)

**Verdict:** signoff

## 1. Commit + branch identity

No code mutations in this turn — this is independent re-verification of the developer's existing commit, not new work. Working tree confirmed clean (`git status --short` → empty).

- **Commit under test:** `2245f1ea3b588393f2ebe605c272e7c0135e4d63` ("fix(ci): take SSL_CERT_FILE from an input, not the 'env' context")
- **Branch:** `forge/workflow/20867f21-b891-4e66-b5bd-7025ddc3716d/run/f6213235-35fb-4e94-a13c-64c8cd8191ba`
- **Remote verified:** `git ls-remote origin <branch>` → `652cfbf1e361d38cb6d8f1e3270ad6c803446791` (matches local HEAD; branch is `forge: character developer phase run idempotency` on top of the fix commit)
- `git log -1 --format="%H" -- .github/actions/deploy-client-stack/action.yml` and `-- .github/workflows/push.yaml` both resolve to `2245f1ea3b` — the fix commit is the latest touch on both real output paths, confirming it's genuinely on this branch (not just claimed).

**Empty-PR-OK:** yes — this turn is verification of an already-pushed fix; no new source changes were needed, and the AC table below carries real command/tool evidence, not hand-waving.

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| — | — | No files mutated this turn (verification-only) |

## 3. Acceptance criteria verification

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | All four actions' `runs.env.SSL_CERT_FILE` no longer references `env.*` | **PASS** | `grep -rnE '\$\{\{[[:space:]]*env\.' .github/actions/*/action.yml` → no output (clean) across all four files, re-run independently this turn. |
| AC2 | Fix uses declared `ssl-cert-file` **input** (valid in `runs.env`), not the `env` context | **PASS** | Re-ran a from-scratch Python/PyYAML structural parse of all four `action.yml` files this turn: `using=docker`, `SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'`, `input_declared=True`, `default='/etc/ssl/certs/ca-certificates.crt'`, `env_refs=[]` for all four → `RESULT: ALL PASS`. |
| AC3 | Default value is safe (not `''`) so Python tooling (gcloud/aws-cli) inside the image still gets a real trust-store path | **PASS** | Confirmed `default: '/etc/ssl/certs/ca-certificates.crt'` present in all four via YAML parse (not just grep on a comment). |
| AC4 | `push.yaml` carries a guard that would have caught the original regression and does not itself break the workflow | **PASS** | Extracted the exact guard body as committed (lines 56–67 of `.github/workflows/push.yaml`) and ran it verbatim under `bash`: against the **current (fixed) tree** → `OK: no 'env' context references in action metadata`, exit 0; against the **pre-fix broken tree** (`git show efbb76d0:...` for all four files) → correctly trips on all four lines with `::error title=env context in action metadata::...`, exit 1. (My first ad-hoc re-implementation of the differential test mis-quoted the `$OPEN` variable and spuriously failed to trip — re-running the *actual* committed script, byte for byte, confirmed it works correctly. Noted so this isn't mistaken for a real defect.) |
| AC5 | All five changed files are valid YAML (no syntax regression) | **PASS** | `yaml.safe_load()` on all 4 `action.yml` + `push.yaml` → `VALID YAML` for each. |
| AC6 | No Go/source files touched — appropriate for a YAML-only CI fix on this constrained host (per CLAUDE.md memory-cap guidance) | **PASS** | `git diff origin/main...HEAD --name-only \| grep -v '^\.github/' \| grep -v '^\.workflow/'` → empty. `make check`/`make build` correctly skipped by the developer; no Go gate applies here. |
| AC7 | api@main consumers no longer fail at "Set up job" (the actual outage this exists to fix) | **FAIL-blocked — operator merge required** | Cannot be live-verified from a branch: GitHub resolves `@main` action refs from the repository's default branch at run time, not from any branch's content. `main` HEAD is still `efbb76d0` (the breaking commit) — confirmed via `read_repository_file(ref=main)`, which still shows `SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}`. The fix is correct and fully verified structurally/statically; the live green run is strictly an operator (merge) step, not something any run on this branch can produce. |
| AC8 | forge-conductor reverts its `main~1` pin to `@main` | **FAIL-blocked — different repo** | Out of scope for `api`; requires a sibling workflow run against `simple-container-com/forge-conductor` after this PR merges. Not attempted — would breach cross-repo write scope. |

## 4. Tests run

No Go code changed — `make check`/`make build` don't apply (per CLAUDE.md, and consistent with the developer's prior turn). Gates actually re-run this turn, independently of the developer's own report:

**a) Clean-gate re-check**
```
grep -rnE '\$\{\{[[:space:]]*env\.' .github/actions/*/action.yml
(no output — clean)
```

**b) Independent structural re-parse (fresh PyYAML run, not reusing dev's script)**
```
deploy-client-stack       using=docker   SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'  input_declared=True default='/etc/ssl/certs/ca-certificates.crt' env_refs=[] OK=True
destroy                   using=docker   SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'  input_declared=True default='/etc/ssl/certs/ca-certificates.crt' env_refs=[] OK=True
cancel-stack              using=docker   SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'  input_declared=True default='/etc/ssl/certs/ca-certificates.crt' env_refs=[] OK=True
provision-parent-stack    using=docker   SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'  input_declared=True default='/etc/ssl/certs/ca-certificates.crt' env_refs=[] OK=True
RESULT: ALL PASS
```

**c) YAML validity re-check (all 5 touched files)**
```
.github/actions/deploy-client-stack/action.yml: VALID YAML
.github/actions/destroy/action.yml: VALID YAML
.github/actions/cancel-stack/action.yml: VALID YAML
.github/actions/provision-parent-stack/action.yml: VALID YAML
.github/workflows/push.yaml: VALID YAML
```

**d) Guard differential test (verbatim extraction, re-run from scratch)**
```
=== vs FIXED (current committed) tree ===
OK: no 'env' context references in action metadata
exit=0

=== vs BROKEN tree (efbb76d0, pre-fix) ===
/tmp/broken1.yml:77:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
/tmp/broken2.yml:80:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
/tmp/broken3.yml:79:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
/tmp/broken4.yml:74:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
::error title=env context in action metadata::The 'env' context is unavailable when GitHub parses action.yml, so every consumer repo fails at 'Set up job' with Unrecognized named-value. Declare an input and reference it as inputs.<name> instead.
exit=1
```

**Skipped:** `actionlint` (not installed on this host, and the developer already reported it does not flag this bug class — not a relevant gate here). `make check`/`make test`/`make build` (no Go files touched).

## 5. Notes for the reviewer

- **A human-authored PR converging on the same bug exists upstream: `simple-container-com/api#625`** ("ci(actions): drop invalid env.SSL_CERT_FILE expression", open, author `smecsia`, created 2026-11-15T21:41Z — before this workflow run started). PR #625 takes the brief's fix **(b)**: it deletes `SSL_CERT_FILE` from `runs.env` entirely on all four actions (net `-16/-0`), relying solely on the image-level CA bake from #624. Our branch instead implements fix **(a)**: keeps `SSL_CERT_FILE` sourced from a new `inputs.ssl-cert-file` with a safe baked-path default, preserving override capability for callers that want a different trust store. **Both resolve the parse-time outage identically** (zero `env.*` references remain either way); they differ only in whether the var stays forwardable. This is an operator-level choice (keep forwarding vs. drop it) — flagging for merge-order awareness, not as a defect in either.
- PR #625 shows `"mergeable_state":"blocked"` (requested reviewers pending, 2 comments, no reviews yet) as of this check — it has not merged ahead of this branch.
- `main` is still on `efbb76d0` (the breaking commit) as of this verification. Whichever fix merges first resolves the live fleet outage described in the brief; the other becomes redundant/conflicting and should be closed by the operator rather than merged on top.
- Re-confirms the developer's own secondary finding: actionlint does not catch this bug class, and no workflow in this repo exercises these composite actions directly — the `push.yaml` guard (grep-based) is the only gate that would catch a reintroduction.

**Verdict:** signoff