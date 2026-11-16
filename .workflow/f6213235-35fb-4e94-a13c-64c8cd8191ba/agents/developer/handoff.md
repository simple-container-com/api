Pushed and confirmed on the remote branch. Here is my handoff.

---

# Developer Handoff — fix `Unrecognized named-value: 'env'` in api's shared GitHub Actions

## 1. Commit + branch identity

- **Commit SHA:** `2245f1ea3b588393f2ebe605c272e7c0135e4d63` (short `2245f1ea3b`)
- **Branch:** `forge/workflow/20867f21-b891-4e66-b5bd-7025ddc3716d/run/f6213235-35fb-4e94-a13c-64c8cd8191ba`
- **Remote verified:** `git log -1 origin/<branch>` → `2245f1ea3b588393f2ebe605c272e7c0135e4d63` (push exit 0)
- **PR URL:** engine auto-opens at handoff time
- **DCO:** exactly one trailer, matching author — `Signed-off-by: David Black <developer@simple-forge.com>`. (First commit attempt picked up the container's `root` identity and produced a second trailer; amended with `--reset-author` after setting `user.name`/`user.email`. Verified: `grep -c "^Signed-off-by:"` = 1.)

## 2. Files touched

| Path | Lines added / removed | What changed |
|---|---|---|
| .github/actions/deploy-client-stack/action.yml | +12/-4 | Added `ssl-cert-file` input (default `/etc/ssl/certs/ca-certificates.crt`); `runs.env.SSL_CERT_FILE` now `${{ inputs.ssl-cert-file }}`; comment rewritten to record the constraint |
| .github/actions/destroy/action.yml | +12/-4 | Same three changes |
| .github/actions/cancel-stack/action.yml | +12/-4 | Same three changes |
| .github/actions/provision-parent-stack/action.yml | +12/-4 | Same three changes |
| .github/workflows/push.yaml | +24/-0 | New `Guard action metadata against the 'env' context` step in the `prepare` job, after checkout |

No Go or other source touched — `git diff --name-only | grep -v '^\.github/'` is empty.

## 3. Acceptance criteria verification

The brief states one defect and prescribes two candidate fixes. Rows below cover each explicit requirement.

| AC# | Criterion | Status | Evidence |
|---|---|---|---|
| AC1 | Remove the `env`-context break from all four actions (deploy-client-stack:77, destroy:80, cancel-stack:79, provision-parent-stack) | **PASS** | `grep -rn 'env\.' $(find .github -name action.yml)` → no output. Confirmed on all four; the four hits that remained after my first edit were in my own *comments* and I reworded them so the grep gate stays meaningful. |
| AC2 | Use fix (a): explicit `ssl-cert-file` input passed through `runs.env`, since `inputs` IS valid there | **PASS** | Structural validator over all four parsed files: `SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'`, `input_declared=True`, `env_context_refs=[]` for each → `RESULT: ALL PASS` (exit 0). Pattern independently proven live: pre-break `2fce01f6` has an inputs-only `runs.env`, and run [43786715794](https://github.com/simple-container-com/forge-conductor/actions/runs/43786715794) pinned to it is `conclusion:success`. |
| AC3 | Fix (b)'s premise — the CA bundle is in the IMAGE, so forwarding is redundant for callers that just need the bundle present | **PASS (verified, and it set the default)** | Against `simplecontainer/github-actions:latest`: `/usr/local/share/ca-certificates/russian-trusted-{root,sub}.crt` present, and SHA-256 fingerprint `D2:6D:2D:…:CF:31` of the baked root **matches cert 119 of 120** inside `/etc/ssl/certs/ca-certificates.crt` (subject `C=RU, O=The Ministry of Digital Development and Communications, CN=Russian Trusted Root CA`). That path is `certFiles[0]` in Go's `crypto/x509/root.go`, so the forwarding was already redundant. |
| AC4 | Don't regress YC TLS for stale callers who still set their own bundle | **PASS** | Input is optional, so a caller may still pass `ssl-cert-file:` explicitly. Default is the baked path, **not** `''` — see the finding below, which is why. |
| AC5 | api@main consumers no longer fail at "Set up job" | **FAIL-blocked — operator merge required** | Cannot be live-verified from a run branch: GitHub resolves `@main` from the default branch, so the proof requires the merge. Pre-state re-confirmed this turn via `read_repository_file(ref=main)` — `main` still carries `${{ env.SSL_CERT_FILE }}`. Static + structural + differential evidence above is complete; the live green run is the operator's post-merge step. |
| AC6 | forge-conductor reverts its `main~1` pin to `@main` | **FAIL-blocked — different repo** | Out of scope for this linked repo. Not attempted: pushing to another repo would breach the cross-repo guard. See cross-repo note below. |

**`cross_repo_dependency`: `simple-container-com/forge-conductor`** — after this PR merges to `api@main`, conductor's three pinned action refs (`simple-container-com/api/...@2fce01f64a8cafef8b24fe755a8982229280c237`, set in `d90757f7c3`) should return to `@main` in `.github/workflows/deploy-forge-conductor.yml` and `destroy-forge-conductor.yml`. Needs a sibling workflow run against that repo.

### Load-bearing finding that changed the implementation

The brief's fix (a) left the default value open. I nearly used `default: ''` (the inert choice) and measured it first:

| `SSL_CERT_FILE` | Python `len(ctx.get_ca_certs())` |
|---|---|
| unset | **120** |
| **empty string** | **0** |
| `/etc/ssl/certs/ca-certificates.crt` | **120** |

An empty default would have silently emptied the file-based trust store for the image's Python tooling (gcloud, aws-cli) in every consumer — a quieter bug than the one I was fixing. Go tolerates empty (`root.go` guards on `certFilePath != ""`), and `openssl verify` ignores the var entirely (its bad-path sanity case also passed), but Python does not. Hence the explicit baked path, which is safe because each action pins its own image.

### Secondary finding

**actionlint does not detect this bug class.** I restored the broken form into a scratch tree and ran actionlint against it: zero context/env findings. Combined with the fact that no workflow in this repo consumes these actions, nothing in CI could have caught `efbb76d079` — the metadata is only ever parsed by downstream repos. That is why I added the grep guard rather than an actionlint gate.

## 4. Tests run

No Go code changed, so `make check` is not the relevant gate (and per CLAUDE.md I avoided linking on this machine). Gates actually run:

**a) Static gate — env-context references in action metadata**
```
=== GATE 1: env-context references in action metadata (must be CLEAN) ===
PASS — zero env-context references

=== GATE 2: input declared + consumed in all four ===
deploy-client-stack        declared=1 consumed=1 default_ok=1
destroy                    declared=1 consumed=1 default_ok=1
cancel-stack               declared=1 consumed=1 default_ok=1
provision-parent-stack     declared=1 consumed=1 default_ok=1
```

**b) Structural validator** (`/tmp/validate_actions.py`, PyYAML, all four files)
```
provision-parent-stack   using=docker
                         SSL_CERT_FILE='${{ inputs.ssl-cert-file }}'
                         input_declared=True default='/etc/ssl/certs/ca-certificates.crt'
                         env_context_refs=[]

RESULT: ALL PASS
validator exit: 0
```

**c) actionlint on `push.yaml`** — my step contributes zero findings
```
=== actionlint on push.yaml: expression/syntax errors ===
.github/workflows/push.yaml:1638:45: property "stack-name" is not defined in object type {version: string} [expression]

=== findings anywhere in my step's range (44-70) — expect EMPTY ===
--- end ---
```
The `:1638` row is pre-existing: `git stash` + re-run reproduced it at `:1614` (same finding, shifted by my +24 lines).

**d) Guard differential test** — body extracted verbatim from the committed YAML, executed under `bash -e` as GitHub would
```
=== A) verbatim guard body vs FIXED tree (expect OK, exit 0) ===
OK: no 'env' context references in action metadata
exit=0

=== B) verbatim guard body vs BROKEN tree (expect trip, exit 1) ===
.github/actions/cancel-stack/action.yml:87:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
.github/actions/deploy-client-stack/action.yml:85:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
.github/actions/destroy/action.yml:88:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
.github/actions/provision-parent-stack/action.yml:82:    SSL_CERT_FILE: ${{ env.SSL_CERT_FILE }}
::error title=env context in action metadata::...
exit=1
```

**Self-inflicted defect caught and fixed during this turn:** my first two guard drafts put the literal `${{` inside the `run:` block (once in code, once in a *comment*). GitHub lexes that token in `run:` bodies, so it would have broken `push.yaml` by the same mechanism as the bug I was fixing. actionlint flagged it both times (`[expression]` at `:57`); final version assembles the token at runtime (`OPEN='$'"{{"`) and keeps it out of the comments too.

**Skipped:** `make check` / `make test` / `make build` — zero Go files touched, and CLAUDE.md records the link needs ~13 GB on this constrained host. `welder run fmt` / `golangci-lint` — no Go files, and they do not lint YAML.

## 5. Notes for the reviewer

- Nothing in this repo's CI exercises these four actions, so post-merge the real signal is the first consumer deploy on `@main`. The guard protects against reintroduction, not against this merge being wrong.
- `branch-preview.yaml:849-861` references the actions only inside a `GITHUB_STEP_SUMMARY` heredoc (docs snippet) — no change needed, since the new input is optional.
- Docs checked for stale CA guidance: `grep -rn 'SSL_CERT_FILE\|russian_trusted\|update-ca-certificates' docs/` → no hits, nothing to update.
- `cancel-stack/action.yml` carries a pre-existing actionlint nit (`unexpected key "options"` on input `stack-type`, line 8) that I left alone — unrelated to this outage and out of scope.

**Verdict:** signoff