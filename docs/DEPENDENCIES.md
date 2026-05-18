# Dependency Selection, Sourcing & Tracking

This document satisfies OpenSSF Baseline **OSPS-DO-06.01** — "the
project documentation MUST include a description of how the project
selects, obtains, and tracks its dependencies."

## Principles

1. **Standard library + first-party SDKs first.** Reach for a third-party
   dep only when the stdlib or an official cloud SDK can't do the job.
2. **Reachability over reputation.** A dep with a published CVE is fine
   if `govulncheck` proves the vulnerable function is never called from
   our code. Conversely, a dep without CVEs is not automatically safe —
   we still SHA-pin and scan it.
3. **Pin everything that crosses a trust boundary.** Hashes for pip,
   `go.sum` for Go modules, SHA digests for Docker base images, commit
   SHAs for GitHub Actions.
4. **No suppressions.** `.trivyignore`, `# noqa`, `// nolint:`, and
   `# nosemgrep` are reserved for documented false positives with a
   reason link in the PR description — not for silencing inconvenient
   findings.

## Ecosystems & how each is tracked

| Ecosystem | Where deps live | Pinning mechanism | Vuln scanning |
|---|---|---|---|
| **Go** | `go.mod` + `go.sum` | `go.sum` hashes every direct + transitive dep | `govulncheck` (reachability-aware), `osv-scanner` (via Scorecard), `trivy fs` |
| **Python (docs)** | `docs/requirements.in` (sources) + `docs/requirements.txt` (compiled with `--generate-hashes`) | `pip install --require-hashes` in `push.yaml` docs-build step | `pip-audit`, Scorecard pinned-deps check |
| **npm (docs examples)** | example `package*.json` files | Lockfile-aware install (`npm ci` when lockfile present, falls back to `npm install`) | Scorecard pinned-deps check |
| **Docker base images** | `Dockerfile`s + `.Dockerfile`s at repo root + example dirs | SHA digest pin: `python@sha256:401f...`, `node:22-alpine@sha256:757e...` | `trivy image` per published image (see HARDENING.md Phase 1) |
| **GitHub Actions** | `.github/workflows/*` + `.github/actions/*` | Commit SHA pin with `# vX.Y.Z` comment for human-readability | Scorecard pinned-deps check; Semgrep custom rules |
| **End-user installer tools** | `sc.sh` (Pulumi installer) | Tarball + SHA256 checksum verification before extract | n/a (sc.sh is shipped, not built against) |

## Selection process

When a new dep is considered:

1. **stdlib check** — does Go / Python stdlib already cover it? If yes,
   reject the dep.
2. **First-party SDK check** — for cloud-related work, prefer the
   official AWS/GCP/Azure/Kubernetes SDK over community wrappers.
3. **Maintenance & community signal** — last release within 12 months,
   open-issue:open-PR ratio not absurd, no orphaned-maintainer signals
   in the dep's repo.
4. **License compatibility** — MIT, Apache-2.0, BSD-3-Clause OK. GPL /
   AGPL flagged for maintainer review (not auto-rejected, but the
   downstream-license implications need to be understood).
5. **CVE history** — `govulncheck` + `osv.dev` lookup on the candidate.
   Active CVEs aren't a blocker if reachability proves we don't hit
   them, but they're declared in the PR.
6. **Transitive blast radius** — how many additional modules does this
   pull in? A 1-line utility with 47 transitive deps is rejected.

## Obtaining & verifying

| Ecosystem | How we obtain | How we verify |
|---|---|---|
| Go modules | `go.sum` hash check on `go mod download` | Module proxy + SHA256 |
| Python (pip) | `pip install --require-hashes -r requirements.txt` — fails if any package missing hash | sha256/sha384 in requirements.txt |
| Docker base images | `docker pull <image>@sha256:<digest>` | Manifest digest |
| GitHub Actions | `uses: owner/repo@<commit-sha>  # vX.Y.Z` | Commit SHA |
| Pulumi (end-user `sc.sh`) | `curl https://github.com/pulumi/pulumi/releases/download/v${VER}/pulumi-v${VER}-linux-x64.tar.gz` + checksum from same release | `sha256sum -c` against checksums.txt from same GitHub Release |

## Update cadence

- **Security advisories**: merged within 24 hours via Dependabot
  security alerts. Branch protection requires CI green + signed
  commits + review before merge — same gates as any other change.
- **Routine version bumps**: Dependabot opens grouped PRs weekly per
  ecosystem (`gomod-minor-and-patch`, `actions-minor-and-patch`,
  `docker-minor-and-patch`, `pip-minor-and-patch`). The grouping config
  also bundles all `simple-container-com/actions/*` bumps into a
  single PR per actions-repo release.
- **Major version bumps**: opened manually (or by Dependabot when the
  group splits them), reviewed with codex + gemini, validated via
  branch-preview build before merge.
- **Base image rebuilds**: every published image is rebuilt on every
  prod release (no float on the digest — the digest itself moves when
  the underlying Dockerfile is touched).

## Scanning & reporting

The PR pipeline runs:

- `go vet`, `staticcheck`, `golangci-lint`
- Semgrep (custom org-policy rules — see `simple-container-com/actions`)
- CodeQL (Go), triggered on all-branch push for full SAST coverage
- `govulncheck` (reachability-aware)
- Trivy image scan on each published Docker artifact
- TruffleHog secret scan on the diff
- Scorecard runs daily; the badge surfaces the current score

A subset of findings is intentionally accepted as documented false
positives — those live in PR descriptions and in
[HARDENING.md](../HARDENING.md), never in a suppression file.

## Out-of-tree dependency surface

Some surfaces consume our artifacts directly:

- `dist.simple-container.com/sc-*.tar.gz` — the `sc` CLI tarballs
- `dist.simple-container.com/sc.sh` — the bootstrap installer
- `simplecontainer/*` Docker Hub images

These are not "deps" in the tree-sense, but the trust chain matters
the same way: each ships with cosign keyless signature + SLSA L3
provenance + SHA256. Consumers verify via the patterns documented in
[SECURITY.md](SECURITY.md).

## Remediation thresholds (SCA policy)

This section satisfies OpenSSF Baseline **OSPS-VM-05.01**
("the project documentation MUST include a policy that defines a
threshold for remediation of SCA findings related to vulnerabilities
and licenses") and is the contract the pre-release gate enforces.

### Vulnerability severity → remediation SLA

| Severity (CVSS or distro rating) | Remediation SLA | Behaviour while open |
|---|---|---|
| **CRITICAL** (9.0–10.0) | within **24 hours** of confirmation if there is a known patched version; within **7 days** if upstream fix pending | merge gate blocks all non-emergency PRs |
| **HIGH** (7.0–8.9) | within **30 days** | merge gate blocks; documented exceptions require VEX (see below) |
| **MEDIUM** (4.0–6.9) | within **90 days** | best-effort with batch bumps via Dependabot |
| **LOW** (0.1–3.9) | next scheduled SCA pass (~weekly) | informational |

The SLA clock starts when the advisory is confirmed reachable in our
codebase by `govulncheck` (reachability-aware). If govulncheck reports
an advisory as *not* reachable, the finding does NOT block — it is
declared `not_affected` via `vex/openvex.json` with the govulncheck
output as evidence (per OSPS-VM-04.02).

### License policy

| License class | Allowed for direct + transitive deps |
|---|---|
| MIT / Apache-2.0 / BSD-3-Clause / BSD-2-Clause / ISC / MPL-2.0 | ✅ Allowed |
| LGPL (any version) | ⚠️ Flagged for maintainer review; case-by-case |
| GPL / AGPL (any version) | ❌ Rejected for runtime; allowed for build-only tooling iff the tool is not redistributed |
| Unknown / proprietary | ❌ Rejected |

License compliance is checked at PR review time. License-incompatible
deps are blocked at merge by maintainer review (no automated license
scanner yet — tracked in HARDENING.md).

## Pre-release SCA gate

This section satisfies OpenSSF Baseline **OSPS-VM-05.02** ("a policy
to address SCA violations prior to any release") and **OSPS-VM-05.03**
("automatically evaluated against a documented policy ... then blocked
in the event of violations").

Production releases are cut automatically on every merge to `main`
(`.github/workflows/push.yaml`), so "pre-release" semantically equals
"pre-merge" — every release IS a merge to main. The SCA gate fires
on every PR + every push to main:

| Tool | Coverage | Blocks on |
|---|---|---|
| **govulncheck** (`.github/workflows/govulncheck.yml`) | Reachable Go vulnerabilities (call-graph aware) | Any reachable advisory finding |
| **CodeQL** (`.github/workflows/codeql.yml`) | Common-vulnerability patterns (SQL injection, XSS, command injection, taint flow, SSRF) | ERROR-severity findings |
| **Semgrep** (`.github/workflows/semgrep.yml`) | Custom rules from the SC org (sigstore, gha-extras, pulumi-iac, go-canon) | ERROR-severity findings |
| **TruffleHog** (`.github/workflows/security-scan.yml`) | Leaked secrets in diff | Any verified secret |
| **Dependabot** | Daily advisory poll → auto-PR for known vulns | Open security alert (visibility, not blocking, but the auto-PR becomes a blocking item per the SLA above) |
| **Trivy / Grype** (in shared security-scan workflow) | SBOM-based scan of source-tree deps | Currently reports counts only; planned to graduate to blocking on HIGH/CRITICAL once VEX consumption is wired in |

### Suppressing a finding (non-exploitable / false positive)

The only sanctioned suppression channel is **VEX**: edit
`vex/openvex.json` and add a `not_affected` statement with:

- `vulnerability.@id` — the OSV / CVE / GHSA ID
- `justification` — one of the OpenVEX standard values
  (`vulnerable_code_not_in_execute_path`,
  `inline_mitigations_already_exist`, etc.)
- `impact_statement` — free-form explanation citing evidence
  (e.g., govulncheck output, code-path analysis, mitigation in place)

`.trivyignore`, `# nosemgrep`, `// nolint:`, `# noqa` are NOT
sanctioned suppression channels. Any of these in a PR must point at
a documented false positive in the PR description; the project
prefers the formal VEX statement for tracking.

Per OSPS-VM-05.03, the suppression flow is also "documented policy":
findings reachable in our code WITHOUT a VEX `not_affected` entry
block the merge. Findings declared `not_affected` in VEX bypass the
gate.
