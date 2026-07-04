# Scoped secret store v1 — implementation spec (`secrets.<scope>.yaml`)

Status: IN PROGRESS — the crypto core + scope model landed in `pkg/api/secrets/scoped`
(scopes.yaml, secrets.<scope>.yaml sealing on sc's own ciphers with scope/key AAD binding,
fail-closed version guard, consistency checks + tests). CLI verbs and deploy-time
resolution wiring follow in this same PR. Implements "Minimal v1" of the keyless-secrets
RFC in this directory.
Prerequisite already shipped: the fail-closed `schemaVersion` store guard is released
and baked fleet-wide, so old binaries hard-fail on formats they do not understand
instead of silently rewriting them.

## Scope decision (settled — panel: Codex + Gemini + 2 Claude lenses)

v1 is deliberately narrow. Three decisions were reviewed against the real consumer
workflows and `pkg/githubactions/actions/parent_repo.go`:

- **D1 — v1 covers PR *scan/lint* jobs ONLY.** The `pull_request` jobs whose secrets move
  into a scope are the pure scanners: `pr-security-scan`, `dast-zap`, `dast-nuclei-ddp`,
  `defectdojo-cleanup`. Their whole secret need is a small low/med set
  (`defectdojo-api-key`, `cf-access-client-id/secret`, `security-triage-slack-webhook-url`,
  `pr-integrail-superadmin-password`). **Deploy-shaped PR jobs stay out of v1**
  (preview deploy, provision-preview, destroy-service): per the parent/child model below,
  dropping their master key forces *every* deploy secret into a scope file — that just
  renames the master key. **Anti-goal made explicit:** the Pulumi crown jewels that
  `pulumi-crossguard-scan` uses on PR (`pulumi-github-aws-*`, config passphrase, github
  token) MUST move to a GitHub-OIDC role (`ci-oidc-pulumi-preview`), **never** into the
  `pr` scope. A scope file that contains deploy-grade credentials is the failure state
  this feature exists to prevent.
- **D2 — `SC_KEY_PR` (age private key in a GitHub Actions secret) ships as the v1 interim,
  with a committed v2.** It is strictly smaller blast radius than today (`SC_CONFIG` opens
  the whole store) and, for a same-org non-fork PR, an OIDC-fetched key would not stop a
  poisoned job from printing what it decrypted — so blocking v1 on v2 buys ~zero
  incremental safety while leaving the whole-store exposure in place. **v2 is not
  optional:** because `sc` already encrypts Pulumi state with AWS KMS and CI already
  federates via OIDC (see `secrets_providers.type=cloud` / `awskms://` in live state),
  the v2 KMS-recipient-via-OIDC path reuses existing infra and is a recipient-list swap on
  the *same* scope files — no v1 work is thrown away. The stored key is retired the moment
  v2 lands.
- **D3 — scope files live in the devops PARENT repo (`integrail` stack store) for v1.**
  Every PR consumer fetches `-s integrail`, and resolution merges parent+child (below), so
  one CODEOWNERS-guarded `secrets.pr.yaml` in the parent serves all consumers. The resolver
  still supports consumer-repo scope files (they merge on top of parent) — that path is
  reserved for the later deploy sweep, not populated in v1.

## Problem

Today one master key (`SIMPLE_CONTAINER_CONFIG`) decrypts the entire whole-file store
(`.sc/secrets.yaml`). Every CI context that needs *any* secret gets *all* secrets —
including `pull_request`-triggered jobs (previews, provision-preview), which are the
most attacker-reachable contexts in the pipeline. Per-scope files with per-scope
recipients replace "one key opens everything" with "a key opens exactly the scope files
it is a recipient of".

## Design summary

- **Crypto/format layer: sc's existing `pkg/api/secrets/ciphers`, NOT SOPS.** The RFC
  named SOPS, but sc depends on neither `getsops/sops` nor `filippo.io/age`, and adding
  them would be a large new supply-chain surface for no capability sc lacks: the ciphers
  package already does per-recipient sealing (RSA-OAEP for `ssh-rsa`, ephemeral-static
  X25519 + ChaCha20-Poly1305 for `ssh-ed25519`) with authenticated encryption. The scoped
  store reuses it, adding only backward-compatible associated-data variants
  (`EncryptLargeStringWithAAD` etc.) — a nil AAD reproduces the legacy wire format
  byte-for-byte, so the whole-file store is untouched.
- **Recipients are SSH public keys** (`ssh-ed25519` / `ssh-rsa`), the same key material the
  whole-file store and `sc secrets allow` already use — not native age recipients. The `pr`
  scope's CI key (`SC_KEY_PR`) is therefore an unencrypted SSH ed25519 private key.
- One committed-encrypted file per scope: `.sc/stacks/<stack>/secrets.<scope>.yaml` — its
  structure (schemaVersion, scope, recipients, value KEYS) is readable/diffable; each value
  is sealed once per recipient, keyed by the recipient's SHA256 SSH fingerprint, values
  opaque. Confidentiality + integrity come from the AEAD/OAEP layer, not a separate MAC.
- v1 recipients are static SSH keys; KMS/OIDC recipients are v2 (a `KeyProvider` that seals
  the same value map to a KMS-wrapped key decrypted via OIDC — a recipient swap, no format
  change).
- The legacy whole-file store keeps working unchanged (mode A). Scoped files are
  additive (mode B); old binaries never open them.

## Parent/child resolution — the load-bearing constraint (P0-1)

A child (client) stack with `parent: <org>/<parent-stack>` does NOT decrypt in isolation.
`parent_repo.go` clones the parent repo, reveals its `.sc/stacks/*` plaintext, **copies it
into the child workspace**, then optionally reveals the child's own `.sc/stacks/<child>/`.
So `${secret:KEY}` resolves against a MERGE of (parent store) ⊕ (child store). Two
consequences the spec must honor:

1. The parent is the real source of shared secrets and the real boundary. v1 scope files
   therefore live in the parent (D3).
2. Any job that drops `SC_CONFIG` and holds only `SC_KEY_PR` can decrypt **only** the
   parent's `secrets.pr.yaml` — it can no longer reveal the parent whole-file store. This
   is exactly why deploy-shaped jobs are excluded from v1 (D1): they would need their whole
   transitive secret set scoped, which recreates a broad key.

## File layout

```
.sc/                                   # (devops PARENT repo, stack `integrail`)
  scopes.yaml                          # scope -> recipients mapping (CODEOWNERS-gated)
  secrets.yaml                         # legacy whole-file registry (mode A, unchanged)
  stacks/integrail/
    secrets.yaml                       # legacy plaintext (gitignored), mode A
    secrets.pr.yaml                    # sc-cipher encrypted, committed, scope "pr"  <-- v1
```

`secrets.<scope>.yaml` files are **committed encrypted**: keys/structure (schemaVersion,
scope, recipients, value names) stay diffable, values are opaque per-recipient blobs. They
are NOT listed in the legacy registry and NOT touched by `sc secrets hide/reveal` legacy
paths. Consumer-repo scope files
(`<consumer>/.sc/stacks/<stack>/secrets.<scope>.yaml`) are a supported location the
resolver merges on top of the parent, reserved for the later deploy sweep — v1 ships only
the parent `secrets.pr.yaml`.

## `scopes.yaml` (the governance surface)

```yaml
schemaVersion: 1
scopes:
  pr:
    description: values safe to expose to pull_request-triggered scan/lint CI
    recipients:
      - ssh-ed25519 AAAA...ci-pr    # ci-pr key (GitHub secret SC_KEY_PR)  [v1 interim]
      - ssh-ed25519 AAAA...admin    # break-glass admin key
      # v2: a KMS-wrapped recipient decrypted via ci-oidc-<pr-scan> — sc re-seals the
      #     same value map to it (updatekeys), then SC_KEY_PR is deleted from GitHub.
```

- **CODEOWNERS-gated** (`/.sc/scopes.yaml @<org>/devops`): recipient changes cannot ride
  an ordinary PR (RFC non-negotiable 3).
- **CODEOWNERS is necessary but not self-enforcing (P0-4):** `sc secrets lint` independently
  verifies each scope file's recipient set == `scopes.yaml` recipients and FAILS on
  drift, so a recipient added by editing a scope file directly (bypassing `scopes.yaml`) is
  caught even if CODEOWNERS review is skipped.
- `sc` regenerates each scope file's recipient set from `scopes.yaml` on
  `allow`/`disallow`/`updatekeys`.
- Removing a recipient re-encrypts the file but does NOT protect history:
  `sc secrets disallow --scope` prints a mandatory rotate-values warning
  (RFC non-negotiable 5).

## CLI UX

```
sc secrets set    --scope pr  -s <stack> KEY [VALUE|-]   # seal/update one value to the scope
sc secrets edit   --scope pr  -s <stack>                 # decrypt-edit-reseal one scope file
sc secrets get    --scope pr  -s <stack> KEY             # decrypt one value
sc secrets reveal                                        # legacy mode A, unchanged
sc secrets allow  --scope pr  <ssh-pubkey>               # update scopes.yaml + reseal (updatekeys)
sc secrets disallow --scope pr <ssh-pubkey>              # ditto + rotate-values warning
sc secrets lint                                          # plaintext-leak + recipient drift + scope/key binding gate
sc secrets doctor                                        # which scopes the ambient key can open
```

Key discovery order for decrypt: `SC_KEY_PR`-style scope key via the ambient
`SIMPLE_CONTAINER_CONFIG`/`SC_CONFIG` private key (CI), or an explicit `--key`. The scope
key is an ordinary SSH private key; being a scope recipient is opt-in per value.

## Scope integrity — binding beyond confidentiality (P0-3)

Per-recipient AEAD/OAEP gives confidentiality + tamper-evidence of each value, but not, by
itself, binding to *where* the value lives. Two attacks are closed by explicit binding
(implemented in `pkg/api/secrets/scoped`):

- **File rename / move:** a PR renames `secrets.prod.yaml` → `secrets.pr.yaml`. Mitigation:
  the scope name is a first-class field inside the file, and `LoadScopeFile` +
  deploy-time resolution FAIL if the in-file scope name does not match the filename's
  `<scope>` (`sc secrets lint` enforces this too).
- **Ciphertext transplant / PR write-poisoning:** a PR copies a `prod`-scoped encrypted
  value blob into `secrets.pr.yaml` to get it decrypted by the PR key. Mitigation: each
  encrypted value's AEAD/OAEP associated data is the domain-separated `scope\0key`, so a
  value blob only decrypts under the exact `(file path, scope, key)` it was written for;
  a transplanted blob fails its AAD check.

## Deploy-time resolution (`${secret:...}`)

For a deploy of environment E of stack S:

1. Resolve E's scope: explicit `secretScope:` in the client stack config, else the
   env name if a scope with that name exists, else stack default, else none.
2. **`secretScope` cannot be raised by a PR (P0-6):** the effective scope for a
   `pull_request`-triggered run is clamped to `pr` (or lower) regardless of what the PR's
   client.yaml says. A PR that sets `secretScope: prod` resolves as `pr` and hard-fails on
   any prod-only `${secret:}` — it can never widen its own scope.
3. Lookup order for `${secret:KEY}`: `secrets.<scope>.yaml` (if the ambient key can
   open it) → legacy mode-A store.
4. **Hard-fail — a real error, not a swallowed warn (P0-2)** if: KEY resolves to nothing;
   or the scope file exists but cannot be decrypted with the ambient key while KEY is not
   in mode A. The resolver returns a non-nil error that aborts the deploy (no
   `logger.Warn(...); continue`); the error names the scope and the missing recipient, and
   the deploy never proceeds partially.
5. A KEY must live in exactly one mode; `sc secrets lint` rejects duplicates
   (mode A vs mode B) to keep resolution deterministic.

## CI wiring (consumer side, Integrail) — v1 = scan/lint only (D1)

- New GitHub secret `SC_KEY_PR` (age private key, recipient of the `pr` scope only).
  The four scan/lint workflows triggered by `pull_request` get `SC_KEY_PR` and **stop
  receiving `SC_CONFIG`**: `pr-security-scan`, `dast-zap`, `dast-nuclei-ddp`,
  `defectdojo-cleanup`.
- One-time `sc secrets set --scope pr` sweep in the parent for the exact keys those jobs
  read: `defectdojo-api-key`, `cf-access-client-id`, `cf-access-client-secret`,
  `security-triage-slack-webhook-url`, `pr-integrail-superadmin-password` — values rotated
  as they move (history hygiene).
- **Out of scope for v1, stated so no one relabels the master key:**
  - deploy-shaped PR jobs (`provision-preview`, `build-and-deploy` PR-preview,
    `destroy-service`) keep `SC_CONFIG` until v2;
  - `pulumi-crossguard-scan`'s Pulumi credentials move to `ci-oidc-pulumi-preview`
    (GitHub-OIDC), **not** the `pr` scope. (A crossguard PR job currently still fetching
    static `pulumi-github-aws-*` is a separate OIDC-migration fix, tracked outside this
    spec.)
- Trusted contexts (push to main / RC/*) keep `SC_CONFIG` until v2; their AWS creds are
  already OIDC-federated (`ci-oidc-sc-deploy`).

## Plaintext-leak lint (ships with the feature, RFC non-negotiable 4)

`sc secrets lint` (and a CI gate in the central security scan):
- every `secrets.<scope>.yaml` parses, every value is a non-empty per-recipient ciphertext
  map (no plaintext value leaf) — `ScopeFile.VerifyConsistency`;
- each value is sealed to exactly the declared `recipients` set (no missing/extra recipient
  fingerprint), and those recipients == `scopes.yaml` recipients for the scope
  (drift = fail) (P0-4);
- in-file scope name == filename `<scope>` (P0-3); value binding (`scope\0key` AAD) is
  enforced structurally at decrypt, not lintable offline;
- no `secrets.<scope>.yaml` is gitignored (must be committed encrypted);
- legacy plaintext files (`stacks/*/secrets.yaml`) remain gitignored (unchanged rule).

## Compatibility & versioning

- Mode A is untouched: format, registry, `hide`/`reveal`, recipients — zero change for
  every current user; nothing is deprecated in v1.
- Old binaries: never open `secrets.<scope>.yaml`; if a value is moved to a scope file
  and an old binary deploys that stack, resolution fails **closed** (missing secret
  hard-fail), never silently empty. `scopes.yaml` carries its own `schemaVersion`,
  covered by the shipped fail-closed guard pattern.
- No new crypto dependency: the scoped store reuses `pkg/api/secrets/ciphers`
  (`golang.org/x/crypto`, already vendored). v1 seals to static SSH recipients; the v2
  KMS/OIDC recipient is an additive `KeyProvider`, not a format or dependency change to the
  file layout.

## Threat-model deltas

| Threat | Before | After v1 |
|---|---|---|
| Compromised PR scan job reads fleet secrets | full store via `SC_CONFIG` | `pr` scope only (low/med scan creds) |
| Deploy-grade creds reachable from a PR scope | n/a | explicitly excluded (D1); crossguard creds → OIDC |
| Malicious PR adds itself as recipient | n/a (single key) | blocked: CODEOWNERS on `scopes.yaml` + `sc` recipient-verify lint (P0-4) |
| PR renames/transplants a scope file | undetected by MAC | scope-name + `path:scope:key` AAD binding (P0-3) |
| PR raises its own `secretScope` | n/a | clamped to `pr`, hard-fail on wider `${secret:}` (P0-6) |
| Missing/undecryptable secret silently empty | possible | real error, deploy aborts (P0-2) |
| Old binary corrupts new format | guarded (schemaVersion) | scoped files never opened by old binaries |
| Recipient removed ≠ revoked | same | explicit rotate-values warning; runbook |
| Stored `SC_KEY_PR` is a smaller master key | — | true, but scoped to low/med scan creds only; retired at v2 (D2, P0-7) |

## Testing

- Unit: scope resolution order, hard-fail matrix (missing key / undecryptable scope /
  duplicate KEY), scopes.yaml↔scope-file recipient drift, scope-name/AAD binding rejection,
  `secretScope` PR-clamp, allow/disallow re-encrypt.
- e2e (preview build, real binary): PR-key can `get --scope pr` but not `--scope prod`;
  a `pull_request` run resolves the four scan jobs' keys from `secrets.pr.yaml` with
  `SC_KEY_PR` and NO `SC_CONFIG`; a PR that renames/transplants a scope file fails lint;
  a PR that sets `secretScope: prod` hard-fails; old released binary against a repo with
  scoped files deploys mode-A-only stacks untouched and hard-fails on a scoped-value stack.
- Panel review (Codex + Gemini + Claude lenses) on the crypto-adjacent surface before
  merge, same as P1.

## Delivery plan (single consolidated PR, after design sign-off)

1. `pkg/api/secrets/scoped/`: scopes.yaml model + scope-file sealing (sc ciphers) + resolution +
   scope-name/AAD binding + `secretScope` PR-clamp + real hard-fail resolver.
2. CLI verbs (`set/edit/get/allow/disallow/lint/doctor` scope forms), with `lint`
   enforcing recipient-verify, path/scope binding, and plaintext-leak gates.
3. Placeholder resolution hook (`${secret:}` order above) wired through parent/child merge.
4. Docs (`secrets-management.md` section) + this spec updated to Status: IMPLEMENTED.
5. Consumer rollout PR (Integrail parent): `secrets.pr.yaml` sweep of the four scan jobs'
   keys (rotated on move) + `SC_KEY_PR` + drop `SC_CONFIG` from the four `pull_request`
   scan workflows. Deploy-shaped jobs and crossguard are NOT touched here.
6. **v2 fast-follow (committed, not optional):** `KeyProvider` KMS recipient decrypted via
   `ci-oidc-<pr-scan>`; `sc secrets allow --scope pr awskms://…`; delete `SC_KEY_PR`.

Open question for review: scope-name↔environment conventions (free-form names vs
enforcing env names), and whether `doctor` should print recipient fingerprints for
audit evidence (ISO/SOC).
