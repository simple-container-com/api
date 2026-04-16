# Container Image Security — Architecture

**Last Updated:** April 15, 2026

## Architecture

```
Build+Push → sign (5s) → verify-image (2s)        ← deploy waits here
               ↑ softFail=false: scan gates sign
Build+Push → scan (~50s, parallel)                ← reports to DefectDojo + step summary
Build+Push → sbom-gen → sbom-att → verify-sbom    ← all 3 artifacts verified
Build+Push → prov-gen → prov-att → verify-prov    ← all 3 artifacts verified
```

All operations use the immutable content digest (`@sha256:...`) returned by push.

## Features

1. **Vulnerability Scanning** — Grype + Trivy dual-tool with merge deduplication
2. **Image Signing** — Cosign keyless (GitHub OIDC) or key-based
3. **Post-Sign Verification** — Immediate cosign verify after signing
4. **SBOM Generation** — Syft CycloneDX with signed attestation + attestation verification
5. **SLSA Provenance** — v1.0 build provenance with signed attestation + attestation verification
6. **Reporting** — GitHub Step Summary, PR comment file, DefectDojo upload

All three artifact types (image signature, SBOM attestation, provenance attestation)
are verified after creation using `cosign verify` / `cosign verify-attestation`.
Verification stdout is redirected to `/dev/null` to prevent Pulumi pipe buffer
deadlocks on large attestation payloads (only exit code matters).

## Design Principles

**Opt-In & Backward Compatible** — all features disabled by default, zero overhead when disabled.

**softFail controls velocity vs enforcement:**
- `softFail: true` (default) — sign runs from push, scan parallel, deploy not blocked
- `softFail: false` — scan gates sign, unsigned images can't deploy (config-only change)

**SC owns its dependencies** — cosign, syft, grype, trivy auto-installed on demand.

**Works in Docker containers** — the `github-actions` binary detects when invoked
via the `sc` symlink and dispatches to the full SC CLI command tree. Security
subcommands (`image sign/scan/verify`, `sbom generate/attach`, `provenance
generate/attach`) work identically in bare runners and Docker action containers.

**Registry auth (cloud-agnostic):** Writes `config.json` from Pulumi `RegistryArgs`
credentials (server + username + password). Uses `resolveStringArg` to handle both
`string` and `*string` types from `sdk.StringPtr` in RegistryArgs. Writes to both
`$HOME/.docker/config.json` and `/root/.docker/config.json` (Docker containers may
differ in HOME vs tool expectations). No cloud-specific logic — all registries
(ECR, GCP Artifact Registry, GHCR, Docker Hub) use the same path.

**PATH propagation** — every Pulumi `local.Command` for security ops prepends
`export PATH="$HOME/.local/bin:/usr/local/bin:$PATH"` because `os.Setenv` in the
Go process does not propagate to Pulumi shell subprocesses. This ensures tools
auto-installed to `~/.local/bin` are findable.

**DefectDojo integration:**
- Engagement: `Container-Scan` for main deploys, `PR-{number}` for PR deploys
- Test title: `Container Scan - {productName}` (stable, no digests/dates — dedup key for reimport)
- Upload uses reimport with `close_old_findings=true` — resolved vulns auto-mitigated

## Reference Docs

- **[Component Design](./component-design.md)** — package structure and interfaces
- **[API Contracts](./api-contracts.md)** — types, function signatures
- **[Integration & Data Flow](./integration-dataflow.md)** — execution flow, file structure, registry auth

## Key Design Decisions

**3. Minimal Invasiveness**
- Leverage external tools (Cosign, Syft, Grype)
- Hook into existing `BuildAndPushImage()` pipeline
- No changes to core Pulumi infrastructure

**4. Configuration-First**
- YAML-based declarative security policy
- Config inheritance from parent stacks
- CLI commands for manual operations

**5. CI/CD Aware**
- Auto-detect CI environment (GitHub Actions, GitLab CI)
- Automatic OIDC configuration for keyless signing
- Support for both automated and manual workflows

---

## Architecture Highlights

### Package Structure

```
pkg/security/
├── config.go           # Security configuration types
├── executor.go         # Main orchestrator
├── context.go          # Execution context with environment detection
├── signing/
│   ├── signer.go       # Cosign wrapper interface
│   ├── keyless.go      # OIDC keyless signing
│   ├── keybased.go     # Key-based signing
│   └── verifier.go     # Signature verification
├── sbom/
│   ├── generator.go    # Syft wrapper interface
│   ├── syft.go         # Syft implementation
│   └── attacher.go     # Attestation attachment
├── provenance/
│   ├── generator.go    # SLSA provenance builder
│   ├── slsa.go         # SLSA v1.0 format
│   └── materials.go    # Build materials collection
├── scan/
│   ├── scanner.go      # Scanner interface
│   ├── grype.go        # Grype implementation
│   ├── trivy.go        # Trivy implementation
│   └── policy.go       # Vulnerability policy enforcement
└── tools/
    ├── installer.go    # Tool installation check
    └── command.go      # Command execution wrapper
```

### Integration Architecture

```
┌─────────────────────────────────────────────────────────┐
│ BuildAndPushImage() - pkg/clouds/pulumi/docker/        │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
         ┌────────────────────────┐
         │  Image Built & Pushed  │
         └───────────┬────────────┘
                     │
                     ▼
         ┌──────────────────────────────────┐
         │  Check SecurityDescriptor Config  │
         └───────────┬──────────────────────┘
                     │
                     ▼
         ┌──────────────────────────────────┐
         │  security.Execute()               │
         │  - Create ExecutionContext        │
         │  - Detect CI environment          │
         │  - Configure OIDC if available    │
         └───────────┬──────────────────────┘
                     │
      ┌──────────────┴──────────────┐
      │                             │
      ▼                             ▼
┌──────────┐                 ┌──────────┐
│  Scan    │  (fail-fast)    │  Skip    │
│  Image   │─────────────X   │  if      │
└────┬─────┘   if critical   │  disabled│
     │         vulns found    └──────────┘
     ▼
┌──────────┐
│   Sign   │
│  Image   │
└────┬─────┘
     │
     ▼
┌──────────┐
│ Generate │
│   SBOM   │
└────┬─────┘
     │
     ▼
┌──────────┐
│  Attach  │
│   SBOM   │
│ Attestat.│
└────┬─────┘
     │
     ▼
┌──────────┐
│ Generate │
│Provenance│
└────┬─────┘
     │
     ▼
┌──────────┐
│  Attach  │
│Provenance│
│ Attestat.│
└────┬─────┘
     │
     ▼
┌──────────────────┐
│ Deployment       │
│ Continues        │
└──────────────────┘
```

### Configuration Schema Extension

```yaml
# In StackConfigSingleImage or ComposeService
security:
  signing:
    enabled: true
    keyless: true  # OIDC-based keyless signing
    verify:
      enabled: true
      oidcIssuer: "https://token.actions.githubusercontent.com"

  sbom:
    enabled: true
    format: cyclonedx-json
    attach: true

  provenance:
    enabled: true

  scan:
    enabled: true
    tools:
      - name: grype
        required: true
        failOn: critical
      - name: trivy
        required: false
```

---

## Key Design Decisions

### 1. External Tools vs Native Implementation

**Decision:** Use external tools (Cosign, Syft, Grype) via command execution

**Rationale:**
- Industry-standard tools with active maintenance
- Avoid reimplementing complex cryptographic operations
- Leverage existing trust in Sigstore ecosystem
- Faster time-to-market

**Trade-offs:**
- External dependencies required
- Command execution overhead (~2-5 seconds per operation)
- Version compatibility management

### 2. Hook Point: Post-Build vs During-Build

**Decision:** Post-build hook after `BuildAndPushImage()` completes

**Rationale:**
- Minimal changes to existing infrastructure
- Clear separation of concerns
- Can operate on already-pushed images
- Supports manual CLI operations on existing images

**Trade-offs:**
- Cannot fail build before push (scanning happens after)
- Requires image to be in registry for attestation

### 3. Fail-Open vs Fail-Closed

**Decision:** Fail-open by default, configurable per feature

**Rationale:**
- Prevents breaking existing workflows
- Encourages adoption without fear
- Users can progressively harden policies
- Security is additive, not disruptive

**Configuration:**
```yaml
scan:
  failOn: critical  # Fail-closed: block on critical vulnerabilities
  warnOn: high      # Fail-open: warn on high vulnerabilities
```

### 4. Keyless (OIDC) vs Key-Based Signing Default

**Decision:** Keyless OIDC signing as default, key-based as fallback

**Rationale:**
- No key management overhead
- Automatic in GitHub Actions
- Transparency log (Rekor) provides audit trail
- Key-based available for air-gapped environments

**Auto-detection:**
```go
// Detect CI environment and OIDC availability
if os.Getenv("GITHUB_ACTIONS") == "true" {
    if os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != "" {
        // Use keyless signing
    }
}
```

### 5. Attestation Storage: Registry vs Separate Store

**Decision:** Store attestations in container registry using OCI artifacts

**Rationale:**
- Co-location with images (same lifecycle)
- Standard OCI artifact format
- Supported by major registries (ECR, GCR, Harbor)
- No additional storage infrastructure

**Compatibility:**
- Requires registry OCI artifact support (most major registries)
- Fallback: store locally only

### 6. Pulumi Resources vs Local Commands

**Decision:** Use `local.Command` Pulumi resources for security operations

**Rationale:**
- Maintains declarative infrastructure-as-code model
- Proper dependency tracking in Pulumi DAG
- Automatic retries and error handling
- Consistent with existing Simple Container patterns

**Example:**
```go
signCmd, err := local.NewCommand(ctx, "sign-image", &local.CommandArgs{
    Create: sdk.Sprintf("cosign sign %s", imageDigest),
}, sdk.DependsOn([]sdk.Resource{image}))
```

### 7. Configuration Inheritance

**Decision:** Security config inherits from parent stacks

**Rationale:**
- Consistent with existing Simple Container patterns
- DRY principle (define once, use everywhere)
- Centralized policy management

**Example:**
```yaml
# Parent stack: .sc/parent-stacks/security-baseline.yaml
security:
  signing:
    enabled: true
    keyless: true

# Child stack: .sc/stacks/myapp/client.yaml
uses: security-baseline
# Inherits signing configuration
```

---

## Performance Considerations

### Expected Overhead (per image)

| Operation | Time | Parallelizable |
|-----------|------|----------------|
| Signing | 5-10s | No (per image) |
| SBOM Generation | 20-30s | Yes (per image) |
| Grype Scan | 30-60s | Yes (per image) |
| Trivy Scan | 30-60s | Yes (per image) |
| Provenance | 2-5s | No (per image) |
| **Total (Sequential)** | **87-165s** | - |
| **Total (Parallel)** | **50-90s** | - |

### Optimization Strategies

1. **Parallel Execution**
   - SBOM generation and scanning run concurrently
   - Multiple images processed in parallel

2. **Caching**
   - Skip scanning if image digest unchanged
   - Cache SBOM for unchanged images
   - Use `~/.simple-container/cache/` directory

3. **Conditional Execution**
   - Skip operations when disabled
   - Early exit on fatal errors (fail-fast scanning)

4. **Tool Selection**
   - Grype: faster, required
   - Trivy: optional secondary validation

---

## Security Considerations

### 1. Key Management

**Private Keys:**
- NEVER stored in Git
- Retrieved from secrets manager (`${secret:cosign-private-key}`)
- Encrypted at rest in secrets manager

**OIDC Tokens:**
- Never logged
- Short-lived (15 minutes)
- Automatically provided by CI platform

### 2. Signature Verification

**Trust Model:**
- Keyless: Trust Fulcio CA + Rekor transparency log
- Key-based: Trust specific public key

**Verification:**
```bash
cosign verify \
  --certificate-identity-regexp "^https://github.com/myorg/.*$" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  docker.example.com/myapp:1.0.0
```

### 3. SBOM Privacy

**Sensitive Information:**
- Exclude private dependencies from public SBOMs
- Local SBOM storage outside Git (`.gitignore`)
- Option to upload to private artifact store only

### 4. Vulnerability Disclosure

**Policy:**
- Critical vulnerabilities block deployment (if configured)
- SBOM includes CVE identifiers
- DefectDojo upload for centralized tracking

---

## Compliance Mapping

### NIST SP 800-218 SSDF

| Practice | Implementation |
|----------|----------------|
| PW.1.3 - Review code | Vulnerability scanning with Grype/Trivy |
| PS.1.1 - Generate SBOMs | Syft SBOM generation in CycloneDX/SPDX |
| PS.3.1 - Archive artifacts | Signed images + Rekor transparency log |
| PS.3.2 - Verify integrity | Cosign signature verification |
| RV.1.1 - Identify vulnerabilities | Dual-toolchain scanning |
| RV.1.3 - Monitor continuously | DefectDojo integration (optional) |

### SLSA Framework

| Level | Requirement | Implementation |
|-------|-------------|----------------|
| Level 1 | Scripted build | Existing `sc` CLI ✅ |
| Level 2 | Version control + signed provenance | SLSA v1.0 provenance attestation |
| Level 3 | Hardened platform + non-falsifiable | Keyless signing with Fulcio + Rekor |

### Executive Order 14028

| Section | Requirement | Implementation |
|---------|-------------|----------------|
| 4(e)(i) | SBOM provision | Syft CycloneDX/SPDX generation |
| 4(e)(ii) | Secure practices | Vulnerability scanning |
| 4(e)(iii) | Provenance | SLSA provenance attestation |

---

## Risk Assessment

### High-Priority Risks

**R-1: External Tool Compatibility**
- **Risk:** Tool version incompatibility breaks workflows
- **Mitigation:** Pin tested versions, graceful degradation
- **Contingency:** Document tested versions, provide installation guides

**R-2: Registry OCI Support**
- **Risk:** Registry doesn't support OCI artifacts
- **Mitigation:** Test major registries (ECR, GCR, Harbor, GHCR)
- **Contingency:** Fallback to local storage only

**R-3: OIDC Token Unavailability**
- **Risk:** CI environment doesn't provide OIDC tokens
- **Mitigation:** Auto-fallback to key-based signing
- **Contingency:** Clear error messages with setup instructions

### Medium-Priority Risks

**R-4: Performance Degradation**
- **Risk:** Security operations slow down deployments significantly
- **Mitigation:** Parallel execution, caching, opt-in
- **Contingency:** Performance profiling, optimization

**R-5: False Positives in Scanning**
- **Risk:** Excessive false positives deter adoption
- **Mitigation:** Dual-toolchain validation, allowlist support
- **Contingency:** Policy tuning documentation

---

## Testing Strategy

### Unit Tests (90%+ coverage)

**Target Packages:**
- `pkg/security/signing/` - Signer implementations
- `pkg/security/sbom/` - SBOM generation
- `pkg/security/provenance/` - Provenance generation
- `pkg/security/scan/` - Scanner implementations

**Mock Strategy:**
- Mock external command execution
- Mock CI environment variables
- Mock registry API calls

### Integration Tests

**Scenarios:**
1. Full workflow: build → scan → sign → SBOM → provenance
2. Keyless signing in GitHub Actions
3. Key-based signing with secrets manager
4. Scan failure blocks deployment
5. Missing tool graceful degradation

### End-to-End Tests

**Test Environments:**
- GitHub Actions workflow
- Local Docker build
- AWS ECR + ECS deployment
- GCP GCR + Cloud Run deployment

---

## Documentation Requirements

### User Documentation

**Guides:**
1. Getting Started with Container Security
2. Configuring Image Signing (Keyless vs Key-Based)
3. SBOM Generation and Management
4. Vulnerability Scanning Policies
5. CI/CD Integration (GitHub Actions, GitLab CI)
6. Troubleshooting Common Issues

**Reference:**
1. Security Configuration Schema
2. CLI Command Reference
3. Compliance Mapping (NIST, SLSA, EO 14028)
4. Registry Compatibility Matrix

### Developer Documentation

**Architecture:**
1. Security Package Design (this document)
2. Integration Points
3. Extension Points (custom scanners, signers)

**Contributing:**
1. Adding New Scanners
2. Testing Guidelines
3. Performance Profiling

---

## Implementation Status

All phases complete. Validated on Everworker (AWS ECR, Blacksmith runners)
and PAY-SPACE (GCP Artifact Registry, SC Docker Action containers).

**Branch:** `simple-forge/issue-93-feature-request-container-imag-ta061x`
**Issue:** https://github.com/simple-container-com/api/issues/93

| Feature | Status | Tested |
|---------|--------|--------|
| Vulnerability scanning (grype + trivy) | Done | EW + PAY-SPACE |
| Image signing (keyless OIDC) | Done | EW + PAY-SPACE |
| Image verification | Done | EW |
| SBOM generation (syft cyclonedx) | Done | EW + PAY-SPACE |
| SBOM attestation + verify | Done | EW |
| SLSA v1.0 provenance + verify | Done | EW |
| DefectDojo upload | Done | EW + PAY-SPACE |
| GitHub Step Summary | Done | EW + PAY-SPACE |
| PR comment file | Done | EW + PAY-SPACE |
| Tool auto-install | Done | EW + PAY-SPACE |
| Docker container compatibility | Done | PAY-SPACE |
| softFail control | Done | EW + PAY-SPACE |

## References

- **Cosign:** https://docs.sigstore.dev/cosign/overview/
- **Syft:** https://github.com/anchore/syft
- **SLSA:** https://slsa.dev/
- **NIST SP 800-218:** https://csrc.nist.gov/publications/detail/sp/800-218/final
