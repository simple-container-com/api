# Scoped secret store v1 — implementation spec (`secrets.<scope>.yaml`)

Status: DRAFT (implements "Minimal v1" of the keyless-secrets RFC in this directory).
Prerequisite already shipped: the fail-closed `schemaVersion` store guard is released
and baked fleet-wide, so old binaries hard-fail on formats they do not understand
instead of silently rewriting them.

## Problem

Today one master key (`SIMPLE_CONTAINER_CONFIG`) decrypts the entire whole-file store
(`.sc/secrets.yaml`). Every CI context that needs *any* secret gets *all* secrets —
including `pull_request`-triggered jobs (previews, provision-preview), which are the
most attacker-reachable contexts in the pipeline. Per-scope files with per-scope
recipients replace "one key opens everything" with "a key opens exactly the scope files
it is a recipient of".

## Design summary (from the RFC decision, unchanged)

- SOPS (`getsops/sops`) is the crypto + format layer: inline value encryption
  (structure readable, leaves opaque), whole-file MAC, partial decrypt, age + KMS
  recipients. No bespoke crypto.
- One readable file per scope: `.sc/stacks/<stack>/secrets.<scope>.yaml`.
- v1 recipients are **age** keys; KMS/OIDC recipients are v2 (`KeyProvider`).
- The legacy whole-file store keeps working unchanged (mode A). Scoped files are
  additive (mode B); old binaries never open them.

## File layout

```
.sc/
  scopes.yaml                          # scope -> recipients mapping (CODEOWNERS-gated)
  secrets.yaml                         # legacy whole-file registry (mode A, unchanged)
  stacks/<stack>/
    secrets.yaml                       # legacy plaintext (gitignored), mode A
    secrets.pr.yaml                    # SOPS-encrypted, committed, scope "pr"
    secrets.staging.yaml               # scope "staging"
    secrets.prod.yaml                  # scope "prod"
```

`secrets.<scope>.yaml` files are **committed encrypted** (SOPS inline): keys/structure
stay diffable, values are opaque. They are NOT listed in the legacy registry and NOT
touched by `sc secrets hide/reveal` legacy paths.

## `scopes.yaml` (the governance surface)

```yaml
schemaVersion: 1.0
scopes:
  pr:
    description: values safe to expose to pull_request-triggered CI
    recipients:
      - age1qq...   # ci-pr key (GitHub secret SC_KEY_PR)
      - age1zz...   # break-glass admin key
  staging:
    recipients: [age1aa..., age1zz...]
  prod:
    recipients: [age1bb..., age1zz...]
```

- **CODEOWNERS-gated** (`/.sc/scopes.yaml @<org>/devops`): recipient changes cannot ride
  an ordinary PR (RFC non-negotiable 3).
- `sc` regenerates each scope file's SOPS recipient set from `scopes.yaml` on
  `allow`/`disallow`/`updatekeys`; a scope file whose SOPS metadata disagrees with
  `scopes.yaml` fails `sc secrets lint`.
- Removing a recipient re-encrypts the file but does NOT protect history:
  `sc secrets disallow --scope` prints a mandatory rotate-values warning
  (RFC non-negotiable 5).

## CLI UX

```
sc secrets set    --scope pr  -s <stack> KEY [VALUE|-]   # add/update one value (SOPS edit)
sc secrets edit   --scope pr  -s <stack>                 # $EDITOR via sops
sc secrets get    --scope pr  -s <stack> KEY             # decrypt one value
sc secrets reveal                                        # legacy mode A, unchanged
sc secrets allow  --scope pr  age1...                    # update scopes.yaml + updatekeys
sc secrets disallow --scope pr age1...                   # ditto + rotate-values warning
sc secrets lint                                          # plaintext-leak + metadata drift gate
sc secrets doctor                                        # which scopes the ambient key can open
```

Key discovery order for decrypt: `SC_AGE_KEY` env (CI) → `SOPS_AGE_KEY_FILE` →
`~/.config/sops/age/keys.txt`. The legacy `SIMPLE_CONTAINER_CONFIG` key is NOT a scope
recipient — scopes are opt-in per value.

## Deploy-time resolution (`${secret:...}`)

For a deploy of environment E of stack S:

1. Resolve E's scope: explicit `secretScope:` in the client stack config, else the
   env name if a scope with that name exists, else stack default, else none.
2. Lookup order for `${secret:KEY}`: `secrets.<scope>.yaml` (if the ambient key can
   open it) → legacy mode-A store.
3. **Hard-fail** (RFC non-negotiable 1) if: KEY resolves to nothing; or the scope file
   exists but cannot be decrypted with the ambient key while KEY is not in mode A —
   error names the scope and the missing recipient, deploy never proceeds partially.
4. A KEY must live in exactly one mode; `sc secrets lint` rejects duplicates
   (mode A vs mode B) to keep resolution deterministic.

## CI wiring (consumer side, Integrail)

- New GitHub secret per scope key: `SC_KEY_PR` (age private key, recipient of `pr`
  scope only). Workflows triggered by `pull_request` get `SC_KEY_PR` and **stop
  receiving `SC_CONFIG`**.
- Values PR jobs actually need (defectdojo API key, docker-registry readonly creds,
  webhook URLs) move into `secrets.pr.yaml` — a one-time `sc secrets set --scope pr`
  sweep per repo, values rotated as they move (history hygiene).
- Trusted contexts (push to main / RC/*) keep `SC_CONFIG` until v2 KMS/OIDC recipients
  land; their AWS creds are already OIDC-federated (`ci-oidc-sc-deploy`).
- This closes the two known PR-shaped holes: PR previews and provision-preview run
  with the `pr` scope key only — a compromised PR job can no longer decrypt the fleet
  store.

## Plaintext-leak lint (ships with the feature, RFC non-negotiable 4)

`sc secrets lint` (and a CI gate in the central security scan):
- every `secrets.<scope>.yaml` parses as SOPS with `sops.mac` present and every value
  leaf `ENC[...]`-armored (`encrypted_regex: '.*'` — whole-leaf encryption by default);
- SOPS metadata recipients == `scopes.yaml` recipients (drift = fail);
- no `secrets.<scope>.yaml` is gitignored (must be committed encrypted);
- legacy plaintext files (`stacks/*/secrets.yaml`) remain gitignored (unchanged rule).

## Compatibility & versioning

- Mode A is untouched: format, registry, `hide`/`reveal`, recipients — zero change for
  every current user; nothing is deprecated in v1.
- Old binaries: never open `secrets.<scope>.yaml`; if a value is moved to a scope file
  and an old binary deploys that stack, resolution fails **closed** (missing secret
  hard-fail), never silently empty. `scopes.yaml` carries its own `schemaVersion`,
  covered by the shipped fail-closed guard pattern.
- SOPS dependency: vendored Go module (`github.com/getsops/sops/v3`), age-only code
  path in v1 (KMS imports build-tagged off until v2).

## Threat-model deltas

| Threat | Before | After v1 |
|---|---|---|
| Compromised PR job reads fleet secrets | full store via `SC_CONFIG` | `pr` scope only |
| Malicious PR adds itself as recipient | n/a (single key) | blocked: CODEOWNERS on `scopes.yaml` + lint drift gate |
| Old binary corrupts new format | guarded (schemaVersion) | scoped files never opened by old binaries |
| Recipient removed ≠ revoked | same | explicit rotate-values warning; runbook |
| Key theft blast radius | everything | one scope; per-scope rotation |

## Testing

- Unit: scope resolution order, hard-fail matrix (missing key / undecryptable scope /
  duplicate KEY), scopes.yaml↔SOPS metadata drift, allow/disallow re-encrypt.
- e2e (preview build, real binary): PR-key can `get --scope pr` but not `--scope prod`;
  deploy of a staging stack resolves mixed mode-A + scoped values; old released binary
  against a repo with scoped files deploys mode-A-only stacks untouched and hard-fails
  on a scoped-value stack.
- Panel review (Codex + Gemini + Claude lenses) on the crypto-adjacent surface before
  merge, same as P1.

## Delivery plan (single consolidated PR, after design sign-off)

1. `pkg/api/secrets/scoped/`: scopes.yaml model + SOPS wrapper (age only) + resolution.
2. CLI verbs (`set/edit/get/allow/disallow/lint/doctor` scope forms).
3. Placeholder resolution hook (`${secret:}` order above) + hard-fail paths.
4. Docs (`secrets-management.md` section) + this spec updated to Status: IMPLEMENTED.
5. Consumer rollout PRs (Integrail): `SC_KEY_PR` + move PR-needed values + drop
   `SC_CONFIG` from `pull_request` workflows.

Open question for review: scope-name↔environment conventions (free-form names vs
enforcing env names), and whether `doctor` should print recipient fingerprints for
audit evidence (ISO/SOC).
