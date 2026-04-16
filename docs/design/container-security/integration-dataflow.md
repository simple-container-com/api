# Integration & Data Flow - Container Image Security

**Last Updated:** April 16, 2026

---

## Integration Point

**File:** `pkg/clouds/pulumi/docker/build_and_push.go`

```
BuildAndPushImage()
  → Build + push image (Pulumi docker.Image)
  → executeSecurityOperations() if security.enabled:
      → createRegistryLogin()       — config.json for security tools
      → createScanCommands()        — grype + trivy (parallel, soft-fail)
      → createSignVerifyCommands()  — cosign sign + verify
      → createSBOMCommands()        — syft gen + cosign att + verify-att
      → createProvenanceCommands()  — slsa gen + cosign att + verify-att
      → security-report             — step summary + PR comment
  → Return ImageOut (deploy depends on verify, not scan)
```

## Dependency Graph

```
softFail=true (default — scan doesn't block deploy):

  push → registry-login → sign → verify-image          ← deploy waits here
  push → registry-login → scan-merged → report         ← parallel, best-effort
  push → registry-login → sbom-gen → sbom-att → verify-sbom
  push → registry-login → prov-att → verify-prov

softFail=false (enforcement — scan gates sign):

  push → registry-login → scan-merged → sign → verify-image
  push → registry-login → sbom-gen → sbom-att → verify-sbom
  push → registry-login → prov-att → verify-prov
```

All operations use the immutable content digest (`@sha256:...`). No mutable tags after push.

## File Structure

```
pkg/clouds/pulumi/docker/
├── build_and_push.go       (637 lines) — core build+push, security orchestration
├── security_helpers.go     (376 lines) — shell utils, CLI args, env, path resolvers
├── security_report.go      (179 lines) — GitHub Step Summary + PR comment generation
└── build_and_push_test.go  (432 lines) — 15 tests

pkg/security/
├── config.go               — SecurityConfig, ScanConfig, PolicyViolationError
├── context.go              — ExecutionContext, OIDC token acquisition
├── executor.go             — Main security executor
├── signing/                — Cosign sign + verify (keyless + key-based)
├── sbom/                   — Syft SBOM generation + cosign attestation
├── provenance/             — SLSA v1.0 provenance generation + attestation
├── scan/                   — Grype + Trivy scanning, policy enforcement
├── reporting/              — DefectDojo upload, SARIF, GitHub reporting
├── tools/                  — Auto-installer for cosign/syft/grype/trivy
└── attestation/            — Attestation parser

pkg/cmd/
├── cmd_image/              — sc image sign|verify|scan
├── cmd_sbom/               — sc sbom generate|attach
└── cmd_provenance/         — sc provenance generate|attach
```

## Registry Auth

Registry credentials flow from Pulumi `RegistryArgs` to `~/.docker/config.json`:

```
RegistryArgs.Password (may be string or *string via sdk.StringPtr)
  → resolveStringArg() handles both types
  → base64(username:password) → config.json
  → Written to $HOME/.docker AND /root/.docker (Docker container compat)
```

Cloud-agnostic — same path for ECR, GCP Artifact Registry, GHCR, Docker Hub.

## Security Report

Generated as a shell script by `buildSecurityReportScript()`, runs after all ops complete:
- Writes markdown table to console + `$GITHUB_STEP_SUMMARY`
- Optionally writes to file for PR comments
- Reads scan results JSON for vulnerability counts + detailed table
- Shows verification status for all 3 artifact types

## DefectDojo Integration

**Engagement routing** (matches Semgrep, Trivy, Grype conventions):

| Trigger | Engagement Name |
|---------|----------------|
| Push to main (staging) | `Source-Scan` |
| PR deploy (pr2209) | `PR-2209` |
| Configured in YAML | Preserved as-is |

**Test title:** `Container Scan - {productName}` — stable dedup key, no digests/dates/tags.

**Reimport logic:** Checks for existing test by title first:
- Found → `POST /api/v2/reimport-scan/` with `test={id}` (updates existing)
- Not found → `POST /api/v2/import-scan/` (creates new test)

Prevents duplicate findings across runs. Uses `close_old_findings=true`.

API key passed via `DEFECTDOJO_API_KEY` env var (Pulumi secret).

## Tool Auto-Install

SC owns all security tool dependencies. `pkg/security/tools/installer.go`:
- Checks if tool is in PATH, installs if missing
- Installs to `/usr/local/bin` (if writable) or `~/.local/bin`
- Adds install dir to Go process PATH via `os.Setenv`
- Pulumi commands prepend `export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"`

Supported: cosign, syft, grype, trivy — with version checking.

## CI/CD Requirements

```yaml
permissions:
  id-token: write  # Required for keyless cosign signing via GitHub OIDC
  contents: write  # For Pulumi state
```

SC Docker Action container includes `sc` symlink to `github-actions` binary.
Tools auto-install on first use — no manual setup required.
