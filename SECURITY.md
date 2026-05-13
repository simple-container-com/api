# Security Policy

Simple Container (`sc`) is an OSS supply-chain tool that runs in consumer
CI/CD and provisions cloud resources in customer accounts. A vulnerability
in this codebase can propagate downstream to every consumer, so we treat
all reports as high priority.

## Supported versions

Security fixes are issued for the **most recent calver release** (the
tag pushed by [`.github/workflows/push.yaml`][push] on every merge to
`main`) and back-ported to the latest `vYYYY.M.x` line only when the
fix is non-trivial. Older versions receive no patches; consumers should
pin to a recent release tag (or a SHA) and update via Dependabot /
[`integrail/devops/.github/actions/install-sc`][install-sc] (or
equivalent) on at least a monthly cadence.

| Version | Supported |
|---|---|
| `vYYYY.M.x` latest | ✅ |
| Previous calver release on the same month line | ✅ (best-effort back-port) |
| Anything older | ❌ |

## Reporting a vulnerability

**Do not file a public issue.** Use one of these channels in order of
preference:

1. **[GitHub Security Advisory][gsa]** — preferred. Private to maintainers,
   integrates with CVE issuance and the GitHub-side fix workflow.
2. **Email** `security@simple-container.com` if you can't use GitHub
   Security Advisories.

Please include:

- A description of the issue and the security impact you observed.
- The exact `sc` version (or commit SHA) affected.
- Reproduction steps or a proof of concept where possible.
- Whether you've shared the report with any third party.

We aim to acknowledge within **3 working days** and to ship a fix or
mitigation within **30 days** for HIGH/CRITICAL findings, **90 days**
for MEDIUM, longer for LOW. We'll keep you updated and credit you in
the advisory unless you ask to remain anonymous.

## Out of scope

These are intentionally outside the scope of this policy because they
sit in the *consumer's* infrastructure, not in this codebase:

- Vulnerabilities in the consumer's cloud account (IAM misconfig, etc.)
  caused by how they *use* `sc`. Reach out to the relevant cloud
  provider or to the consumer.
- Vulnerabilities in third-party tools `sc` invokes (`pulumi`, `cosign`,
  `syft`, `trivy`, `grype`, `gcloud`, `kubectl`). Report those upstream.
- The Caddy / cloud-helpers / kubectl Docker images' *upstream* OS
  packages — we re-roll on each release and the deferred CVE log is
  documented in PRs at merge time.

## Hardening posture

The repository is hardened against the relevant supply-chain risks
covered by CIS, OWASP CICD Top 10, SLSA, NIST SSDF, and the OpenSSF
Scorecard. Current control status is tracked in the hardening pipeline
(image scan, SBOM, Semgrep, Dependabot, secret scan run on every PR
and merge). For details on the threat model and the controls that
ship with each release, see the PR history.

## Cryptographic primitives

`sc` uses **only** primitives from the Go standard library and a small
set of audited libraries (`cosign`, `sigstore-go`). We avoid rolling
our own crypto. The local security-scan cache uses HMAC-SHA256 with a
32-byte random per-cache key for tamper detection.

[push]: .github/workflows/push.yaml
[install-sc]: https://github.com/simple-container-com/actions/tree/main/install-sc
[gsa]: https://github.com/simple-container-com/api/security/advisories/new
