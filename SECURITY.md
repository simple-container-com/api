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

## Artifact signing and verification (Phase 2)

Every release produces signed, attested artifacts published to Docker
Hub and `dist.simple-container.com`. Consumers can verify before use.

### Identity-regex contract

Cosign keyless signatures bind the signing identity to a GitHub
Actions OIDC subject. Consumers verify against one of two pinned
identities; **do not mix them**.

| Trust root | Subject regex | Use for |
|---|---|---|
| **Production** | `^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$` | `sc.sh` installs; production Docker images (`:latest`, `:vYYYY.M.x`, `:aws-vYYYY.M.x`); release tarballs |
| **Staging** | `^https://github\.com/simple-container-com/api/\.github/workflows/build-staging\.yml@refs/heads/staging$` | Consumers who **knowingly opt in** to `:staging` images via composite actions |
| OIDC issuer (both) | `https://token.actions.githubusercontent.com` | — |

If either workflow file is ever renamed, the regex above is
bumped in the same PR. This file is the canonical reference for
consumer-side verification.

### Verifying images

Always verify by digest, not tag — tags are mutable. SLSA build
provenance is verified via the GitHub-native `gh attestation verify`
because we publish provenance through `actions/attest-build-provenance@v4`
(a Sigstore bundle, not a raw `intoto.jsonl`).

```bash
IMG=docker.io/simplecontainer/github-actions
DIGEST=$(crane digest "$IMG:vYYYY.M.x")   # pin to the immutable digest
cosign verify "$IMG@$DIGEST" \
  --certificate-identity-regexp '^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
cosign verify-attestation "$IMG@$DIGEST" --type cyclonedx \
  --certificate-identity-regexp '^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
gh attestation verify "oci://$IMG@$DIGEST" --repo simple-container-com/api
```

### Verifying tarballs

The CDN ships these sidecars next to every tarball:

- `<tarball>.sha256` — SHA-256 checksum
- `<tarball>.cosign-bundle` — cosign keyless bundle (cert + sig + Rekor entry)
- `<tarball>.sigstore.json` — SLSA build provenance (Sigstore bundle from `attest-build-provenance@v4`)

```bash
T="sc-linux-amd64-vYYYY.M.x.tar.gz"
curl -fLO "https://dist.simple-container.com/$T"{,.sha256,.cosign-bundle,.sigstore.json}
sha256sum -c "$T.sha256"
cosign verify-blob --bundle "$T.cosign-bundle" \
  --certificate-identity-regexp '^https://github\.com/simple-container-com/api/\.github/workflows/push\.yaml@refs/heads/main$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com "$T"
gh attestation verify "$T" --bundle "$T.sigstore.json" \
  --repo simple-container-com/api
```

`sc.sh` will run the tarball steps automatically when `cosign` is on
`PATH` — that integration lands in the follow-up PR (see
[`HARDENING.md`](../HARDENING.md) Phase 2 plan; until it merges, the
commands above are the manual verification path).

### Composite-action consumers — SHA-pin the underlying image

`simple-container-com/api/.github/actions/{deploy-client-stack,
provision-parent-stack,destroy,cancel-stack}` are docker-action
wrappers that pull `simplecontainer/github-actions:staging` by **tag**
at consume-time. Tags are mutable; the underlying image is signed but
the GitHub Actions runtime does not verify the signature before
launching the container.

Consumers running these actions in **production** pipelines should
pin the action repository **and** the docker image to a digest. The
recommended pattern (see `simple-container-com/actions` for the
maintained variant of these wrappers):

1. Pin the action ref by SHA, not `@main`.
2. Vendor the action.yml locally and replace
   `image: 'docker://simplecontainer/github-actions:staging'` with
   `image: 'docker://simplecontainer/github-actions@sha256:<digest>'`
   for the digest you have verified out-of-band with `cosign verify`.
3. Re-bump the digest on a documented cadence (we publish the
   current production digest in every release-notes entry).

A native `cosign verify` step inside the wrapper action is on the
roadmap; until then, **digest-pinning is the only consumer-side
mitigation for the mutable-tag pull path**.

### Residual risk: CDN rollback

A network attacker who can rewrite responses from
`dist.simple-container.com` can serve an older, validly-signed,
still-vulnerable tarball when the consumer fetches the unversioned
`sc-os-arch.tar.gz` pointer. The signature still verifies (the older
build was legitimately signed at release time) but the binary is
known-vulnerable.

Mitigation in this phase: `sc.sh` (Phase-2 PR 2c) defaults to
fetching the **latest version** from a signed `version` manifest,
not the unversioned tarball. Consumers who set
`SIMPLE_CONTAINER_VERSION=vYYYY.M.x` get the explicit version they
asked for; consumers who do not set it get the version the manifest
declares current.

This residual risk is closed by TUF/RSTUF in Phase 6.

[push]: .github/workflows/push.yaml
[install-sc]: https://github.com/simple-container-com/actions/tree/main/install-sc
[gsa]: https://github.com/simple-container-com/api/security/advisories/new
