# Maintainers

This document satisfies OpenSSF Baseline **OSPS-GV-01.01** (list of
project members with access to sensitive resources) and **OSPS-GV-01.02**
(roles and responsibilities for project members).

## Active maintainers

| Person | GitHub | Role | Areas of focus |
|---|---|---|---|
| **Dmitrii "Creed"** | [@creeed22](https://github.com/creeed22) | DevSecOps Lead, Security Contact, Release Manager | Supply-chain hardening, release pipeline, security policy, downstream consumer support (Integrail/PAY-SPACE/EverWorker) |

> **Note**: this list reflects the maintainers actively merging to
> `main` at the time of writing. The canonical machine-readable source
> is `.github/CODEOWNERS` — when a path-specific reviewer is required
> for merge, that file controls.

## Roles

### Maintainer
- Merges PRs to `main` (subject to branch protection: signed commits,
  review approval, status checks green)
- Responds to security reports through the channels in
  [`SECURITY.md`](SECURITY.md)
- Cuts production releases via the automated `push.yaml` workflow on
  merge to `main` (welder pushes the tag; GitHub Release is created
  automatically per [`scripts/create-github-release.sh`](../scripts/create-github-release.sh))
- Triages community issues + PRs

### Reviewer (no current dedicated reviewers)
- Reviews PRs without merge access; a Maintainer must approve the
  final merge

### Contributor
- Opens PRs per the rules in [`CONTRIBUTING.md`](CONTRIBUTING.md)
- No special access — just GitHub fork + PR flow

## Sensitive-resource access map

The following resources have access controlled by Maintainer accounts.
Compromise of any of these would put consumer artifacts at risk —
hence the threat model in [`SECURITY.md`](SECURITY.md) + the cosign
signing chain that lets consumers verify artifacts independently.

| Resource | Why it's sensitive | Held by |
|---|---|---|
| **GitHub** `simple-container-com` org admin | Repository settings, branch protection, member management | Maintainers (currently Dmitrii) |
| **GitHub Actions secrets** in `simple-container-com/api` | `SC_CONFIG`, Docker Hub publish token, GitHub App tokens used in CI | Maintainers |
| **Docker Hub** `simplecontainer` org admin | Publishes images consumers pull via `sc deploy` | Maintainers |
| **GCS bucket** `simple-container-api--dist--prod` | Hosts `sc.sh` + tarballs at `dist.simple-container.com` | Maintainers |
| **Cloudflare** `simple-container.com` zone | DNS for `dist.*`, `docs.*`, `welder.*`, etc.; WAF rules; DNSSEC | Maintainers |
| **NameCheap** registrar for `simple-container.com` | DNSSEC DS record; domain renewal | Maintainers |
| **AWS Secrets Manager** for SC infra credentials | Pulumi state encryption keys, Telegram CI/CD bot token, etc. | Maintainers |
| **bestpractices.dev** project 12886 | OpenSSF Baseline attestation | Maintainers |

## Adding or removing a maintainer

Changes to this list happen via a PR amending this file + a
corresponding update to:

- `.github/CODEOWNERS` (if path-scoped review duties)
- The relevant resource ACL (GitHub org members, Docker Hub team,
  Cloudflare account members, etc.)

Maintainer offboarding additionally:
- Rotates any shared CI tokens / API keys the departing maintainer
  could access (Docker Hub publish token, Cloudflare API tokens,
  GitHub PAT-equivalents)
- Revokes Sigstore / cosign signing identities if the maintainer's
  GitHub workflow identity was wired into any signing path
- Audits the org-level 2FA enforcement (per
  [HARDENING.md](../HARDENING.md) Phase 8 admin-UI list)

## Security contact

For responsible-disclosure security reports, see [`SECURITY.md`](SECURITY.md).
Preferred channel is GitHub Security Advisory; email fallback is
`security@simple-container.com` (group) or `creed@simple-container.com`
(direct).

## Project communication

- **Public issues / PRs**: this repo
- **Discussions**: this repo's Discussions tab
- **Security (private)**: GHSA / email per `SECURITY.md`
- **Downstream coordination** (Integrail, PAY-SPACE consumers): direct
  channels held by maintainers, not part of public-facing project
  governance
