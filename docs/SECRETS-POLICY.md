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
  contain secrets. A consumer's cloud credential is not a known value
  to GitHub, so it is handled by the rule below instead.
- **Job-scoped env**: secrets passed to steps via `env:` at the step
  level (or `env:` on the smallest enclosing job), never globally.

## Credentials Simple Container passes to Pulumi

A consumer's cloud credential does not stay in the config file it came
from: Simple Container hands it to a Pulumi provider, and from there it
is recorded in the stack checkpoint and rendered in preview and update
diffs. A dry-run in CI therefore prints whatever was passed in clear.

Two rules keep that closed.

- **Mark the input secret at the call site.** Generated provider SDKs
  wrap the fields they own (pulumi-aws `accessKey`/`secretKey`/`token`,
  pulumi-gcp `accessToken`, pulumi-cloudflare `apiToken`,
  pulumi-mongodbatlas `privateKey`, the `data` of a Kubernetes
  `Secret`), and `TestUpstreamProvidersStillSecretTheirOwnCredentials`
  fails if an SDK bump stops holding up its end. Several fields Simple
  Container uses are **not** among them and are wrapped here, with
  `pApi.SecretString`: gcp `credentials`, kubernetes `kubeconfig`,
  docker `registry.password`, a `command:local:Command` environment or
  script that carries one, and a Cloudflare worker script that embeds a
  basic-auth password.

  Adding a call site that hands a credential to Pulumi means wrapping it
  there too. The rule is per call site rather than per type, so it is
  worth grepping for `pApi.SecretString` next to whatever you are adding
  rather than assuming the class is covered.
- **Redact on the way out.** `PreviewResult.Summary`,
  `UpdateResult.Summary`, provider diagnostics and the errors returned
  from a failed operation pass through `redactCredentials`, which
  removes PEM private keys and credential-named scalar fields from
  engine output. This is defence in depth; it is not a substitute for
  the first rule, and it has known limits: a credential inside an array,
  one embedded in a connection URI, and one in a bare all-letter value
  on a diagnostic line are not removed.

A credential printed in clear by `sc provision`, `sc deploy` or a
preview in CI is a leak, and is handled under **On suspected leak**
below: rotate first, then fix the call site. A checkpoint written before
the fix keeps its plaintext copy, in the state file and in the retained
backup generations, until the stack is deployed again.

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
