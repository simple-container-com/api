# Secrets & Credentials Policy

This document satisfies OpenSSF Baseline **OSPS-BR-07.02** —
"the project MUST define a policy for managing secrets and credentials
used by the project. The policy should include guidelines for storing,
accessing, and rotating secrets and credentials."

It complements [`SECURITY.md`](SECURITY.md) (threat model + disclosure)
and [`MAINTAINERS.md`](MAINTAINERS.md) (who holds what).

## Categories of secret

| Category | Examples | Where they live |
|---|---|---|
| **CI/CD secrets** | `SC_CONFIG` SC stack configuration, Docker Hub publish token, Telegram CI bot token | GitHub Actions encrypted secrets, repo-scoped |
| **Production runtime secrets** | DB credentials, Pulumi state encryption keys, customer-supplied cloud credentials | Consumer-side AWS Secrets Manager / GCP Secret Manager (NEVER in this repo) |
| **External-platform admin tokens** | Cloudflare API tokens, NameCheap API access, GitHub PATs for cross-repo access | Maintainer-personal vaults (1Password, similar); never committed |
| **Sigstore signing identity** | The build workflow's GitHub OIDC token used by cosign keyless sign | Ephemeral; minted per-build by GitHub Actions, valid ~10 min |
| **Test fixtures with placeholder credentials** | Example secrets YAML in `docs/docs/examples/secrets/`, `pkg/api/secrets/testdata/` | Repo, but in a format that TruffleHog excludes (per `.github/workflows/security-scan.yml` `secret-scan-extra-excludes`) |

## Storing secrets

- **Never commit** real secrets to the repo. TruffleHog runs on every
  PR diff in `.github/workflows/security-scan.yml` and fails the build
  on detected findings (`fail-on-secrets: true`). GitHub
  secret-scanning push-protection is also enabled at the repo level.
- **Encrypted-at-rest only**: CI secrets in GitHub Actions; production
  secrets in AWS Secrets Manager or GCP Secret Manager (consumer side).
- **No plain-text checked-in encrypted secrets**: even SOPS/Mozilla
  encrypted-blobs in the repo are discouraged. Use the platform's
  native secret store.
- **Example / test fixtures** must use clearly-non-credential
  placeholders (`<your-token-here>`, `xxxxxxxxxxxxxxx`). Where the
  fixture's format requires a real-shaped string (e.g., OpenSSH key
  body in `pkg/api/secrets/testdata/`), the file is excluded via the
  central TruffleHog excludes list in the shared security-scan
  workflow.

## Accessing secrets

- **Principle of least privilege per CI job**: workflows set root
  `permissions:` to `contents: read`; per-job `write` only where
  required (e.g., `docker-finalize` for tag push + Release create).
  Scorecard Token-Permissions = 10/10 after PR #263 enforces this.
- **No `pull_request_target`** on workflows that handle untrusted PR
  code. The shared security scan uses `pull_request` so fork PRs run
  with a read-only token and no access to org secrets.
- **No secrets in workflow logs**: GitHub Actions auto-redacts known
  secret values; we add no debug echoes of env vars that might
  contain secrets.
- **Job-scoped env**: secrets passed to steps via `env:` at the step
  level (or `env:` on the smallest enclosing job), never globally.

## Rotation cadence

| Secret type | Cadence | Trigger |
|---|---|---|
| **CI publish tokens** (Docker Hub, Sigstore-key alternatives if any) | 90 days | Calendar reminder; immediate on suspected leak |
| **Cloudflare API tokens** (Maintainer-personal) | Quarterly | Calendar; immediate on maintainer offboarding |
| **GitHub Personal Access Tokens** (for cross-repo automation) | 90 days, scoped to specific repos only | Calendar |
| **Sigstore Fulcio certs** (per-build) | Per-build (~10 min lifetime) | Automatic |
| **Production runtime secrets** (DB creds, etc.) | Per consumer's policy | Owned by consumer |

Rotation is recorded in the maintainer's credential inventory (kept
in a private SC team vault, not published here per
[`MAINTAINERS.md`](MAINTAINERS.md) "Sensitive-resource access map").

## On suspected leak

1. **Rotate immediately** — don't wait for the next scheduled cadence.
2. **Audit the access logs** of the platform the secret was on
   (GitHub Audit Log, Cloudflare Audit Log, Docker Hub access log,
   AWS CloudTrail, etc.). Cloudflare audit-log retention is currently
   18 months on the SC `simple-container.com` account (Free-tier
   default, satisfies OpenSSF ≥1-year requirement).
3. **Open a private security advisory** via the channel in
   [`SECURITY.md`](SECURITY.md) if the leak affects a published artifact.
4. **Document the incident** in the post-mortem PR description (no
   secret values, just the rotation reference + access-log summary).

## Detection

- **TruffleHog** on every PR diff (`.github/workflows/security-scan.yml`
  via the shared `simple-container-com/actions/.github/workflows/security-scan.yml`).
- **GitHub secret-scanning** with **push-protection**: blocks pushes
  that contain recognised credential formats.
- **Semgrep** custom rules (`simple-container-com/actions/semgrep-scan`)
  flag patterns like hardcoded JWTs, AWS access keys in code,
  embedded private keys.

## Cross-references

- [`SECURITY.md`](SECURITY.md) — threat model + disclosure
- [`MAINTAINERS.md`](MAINTAINERS.md) — who holds maintainer-personal
  credentials
- [`DEPENDENCIES.md`](DEPENDENCIES.md) — dep policy
