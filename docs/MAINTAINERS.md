# Maintainers & Contributors

This document satisfies OpenSSF Baseline **OSPS-GV-01.01** (list of
project members with access to sensitive resources) and **OSPS-GV-01.02**
(roles and responsibilities for project members).

## Active maintainers

Listed in descending order of total commit count to `main` at the
time of writing. Each maintainer has merge access; sensitive-resource
access is shared per the access map below.

| Person | GitHub | Role |
|---|---|---|
| **Ilia Sadykov** | [@smecsia](https://github.com/smecsia) | Project lead, core maintainer |
| **Universe Ops** | [@universe-ops](https://github.com/universe-ops) | Core maintainer — features + cloud integrations |
| **Dmitrii "Creed"** | [@Cre-eD](https://github.com/Cre-eD) | DevSecOps lead, security contact, release-pipeline owner, downstream-consumer support (Integrail / PAY-SPACE / EverWorker) |

## Active contributors

| Person | GitHub | Notes |
|---|---|---|
| **Bao Tran** | [@baotn166](https://github.com/baotn166) | Recurring contributor |

> The canonical machine-readable source for path-scoped review
> ownership is [`.github/CODEOWNERS`](../.github/CODEOWNERS). When a
> path-specific reviewer is required for merge, that file controls.

## Historical contributors

These contributors have one or more commits in `main`'s history. We
list them in `git shortlog` order, omitted from the formal maintainer
list because they are not currently active in the merge cadence:

- Andrey Krasavin — early contribution

## Bot accounts

These automate the contribution flow and are not human members. They
do not have access to sensitive resources beyond what the workflow they
run inside grants them, scoped per-job by GitHub Actions permissions.

- `simple-container-forge[bot]` — opens automated PRs for cross-repo
  forge work
- `blacksmith-sh[bot]` — CI infrastructure (Blacksmith runners)
- `dependabot[bot]` — opens dependency-bump PRs per `.github/dependabot.yml`

## Roles

### Maintainer
- Merges PRs to `main` (subject to branch protection: signed commits,
  review approval, status checks green, DCO sign-off enforced by
  `.github/workflows/dco.yml`)
- Responds to security reports through the channels in
  [`SECURITY.md`](SECURITY.md)
- Production releases are cut automatically on every merge to `main`
  via `push.yaml`: `welder run tag-release` pushes the calver tag,
  `welder deploy -e prod` publishes to `dist.simple-container.com`,
  and [`scripts/create-github-release.sh`](../scripts/create-github-release.sh)
  attaches the signed sidecars (`.sigstore.json` / `.cosign-bundle` /
  `.sha256` / `.sbom.cdx.json`) to the GitHub Release.
- Triages community issues + PRs.

### Contributor
- Opens PRs per the rules in [`CONTRIBUTING.md`](CONTRIBUTING.md).
- No special access — standard GitHub fork + PR flow.
- See `CONTRIBUTING.md` for the security-sensitive-change protocol
  that pulls in additional reviewers for `pkg/security/` / `push.yaml` /
  `sc.sh` changes.

### Decision-making

This section satisfies the OpenSSF Best Practices `governance`
criterion (how decisions on technical direction are made).

- **Default rule: consensus via PR review.** Any change ships only
  after a maintainer's approving review on the PR and all required
  status checks (CI, signed commits, DCO sign-off) pass. Branch
  protection on `main` enforces this — no maintainer self-merges
  unreviewed work.
- **Security-sensitive changes** (`pkg/security/`, `push.yaml`,
  `sc.sh`, anything in the SLSA / cosign / Sigstore chain) require at
  least one maintainer review on top of the automated codex + Gemini
  review pass triggered by `/thorough-review`. The contribution rules
  in [`CONTRIBUTING.md`](CONTRIBUTING.md) describe this protocol.
- **Contentious technical decisions** (architecture, breaking changes,
  security-policy shifts) are resolved by maintainer discussion in the
  PR thread or repo Discussions. If consensus is not reached, the
  project lead (currently [@smecsia](https://github.com/smecsia)) has
  tiebreaker authority. Material direction changes are surfaced in the
  Discussions tab before they ship.
- **Roadmap shaping** follows the same pattern — see
  [`ROADMAP.md`](ROADMAP.md) for how issues become shipped code, and
  how maintainers triage them.

## Sensitive-resource access map

The following resources have access controlled by the maintainer
group. Compromise of any of these would put consumer artifacts at risk
— hence the threat model in [`SECURITY.md`](SECURITY.md) + the cosign
signing chain that lets consumers verify artifacts independently.

| Resource | Why it's sensitive | Held by |
|---|---|---|
| **GitHub** `simple-container-com` org admin | Repo settings, branch protection, member management | Maintainer group |
| **GitHub Actions secrets** in `simple-container-com/api` | `SC_CONFIG`, Docker Hub publish token, GitHub App tokens used in CI | Maintainer group |
| **Docker Hub** `simplecontainer` org admin | Publishes images consumers pull via `sc deploy` | Maintainer group |
| **GCS bucket** `simple-container-api--dist--prod` | Hosts `sc.sh` + tarballs at `dist.simple-container.com` | Maintainer group |
| **Cloudflare** `simple-container.com` zone | DNS for `dist.*`, `docs.*`, `welder.*`; WAF rules; DNSSEC | Maintainer group |
| **NameCheap** registrar for `simple-container.com` | DNSSEC DS record; domain renewal | Maintainer group |
| **AWS Secrets Manager** for SC infra credentials | Pulumi state encryption keys, Telegram CI/CD bot token, etc. | Maintainer group |
| **bestpractices.dev** project 12886 | OpenSSF Baseline attestation | Maintainer group |

The principle of least privilege applies internally — each maintainer
holds only the credentials needed for the work they do. Specific
per-maintainer ACL membership is tracked in the SC team's internal
credential inventory (not published here to avoid leaking attack
surface, but maintained per `SECURITY.md`'s threat model).

## Promoting a contributor / Granting escalated permissions

This section satisfies OpenSSF Baseline **OSPS-GV-04.01** —
"the project documentation MUST have a policy that code collaborators
are reviewed prior to granting escalated permissions to sensitive
resources."

A contributor is considered for maintainer-role promotion only after
the following review-and-vetting gates are passed. Each gate is
recorded in the promotion PR description.

### Track-record gate

- **Minimum**: 5+ merged PRs over ≥3 months, spanning multiple files
  / packages.
- Demonstrated review-and-iterate behaviour (responsive to reviewer
  feedback, clean commit history, signed commits with DCO sign-off).
- No suppression-policy violations in merged PRs (`.trivyignore`,
  `# nosemgrep`, `// nolint:` etc. used outside the sanctioned VEX
  channel documented in [DEPENDENCIES.md](DEPENDENCIES.md)).

### Sponsorship gate

- An existing maintainer must **explicitly nominate** the contributor
  in a PR amending this file, with rationale.
- A second maintainer (or, if only one maintainer exists, the project
  lead) seconds the nomination.
- The contributor accepts in the PR thread.

### Account-hardening gate (must pass BEFORE any resource ACL changes)

The promoting maintainer verifies and records (in the promotion PR
description) that the candidate's accounts meet:

- ✅ **2FA enabled** on GitHub (TOTP or hardware key — NOT SMS).
- ✅ **2FA enabled** on Docker Hub if they will receive `simplecontainer`
  org access.
- ✅ **2FA enabled** on Cloudflare if they will receive zone admin.
- ✅ **SSH/GPG commit-signing key** is registered in GitHub and matches
  the GitHub account identity (no impersonation surface). Signed
  commits will be required on every PR they merge.

### Least-privilege grant

- Even after promotion, the new maintainer receives **only the
  credentials needed for their work**. Per the SECURITY.md threat
  model, not every maintainer holds every key.
- Specifically: Docker Hub publish access is granted only if the
  maintainer regularly handles releases; Cloudflare admin only if
  they own DNS/WAF; AWS Secrets Manager only if they own the
  infrastructure that consumes those secrets.
- Specific per-credential ACL membership is recorded in the SC team's
  internal credential inventory (not in this public file) per
  [SECRETS-POLICY.md](SECRETS-POLICY.md).

### Probationary period

- For the first 30 days after promotion, the new maintainer's merges
  are **co-reviewed** by an existing maintainer — they may approve,
  but another maintainer must also approve before merge. After 30
  days with no concerns, full merge authority becomes effective.

## Adding or removing a maintainer (mechanics)

Changes to this list happen via a PR amending this file + a
corresponding update to:

- [`.github/CODEOWNERS`](../.github/CODEOWNERS) (if path-scoped review
  duties)
- The relevant resource ACL (GitHub org members, Docker Hub team,
  Cloudflare account members, etc.)

Maintainer offboarding additionally:

- Rotates any shared CI tokens / API keys the departing maintainer
  could access (Docker Hub publish token, Cloudflare API tokens,
  GitHub PAT-equivalents) — per [SECRETS-POLICY.md](SECRETS-POLICY.md)
  rotation schedule, accelerated to "immediate" on offboarding.
- Revokes Sigstore / cosign signing identities if the maintainer's
  GitHub workflow identity was wired into any signing path.
- Audits org-level 2FA enforcement against the maintainer-side admin
  checklist.

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
