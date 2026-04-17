# Container Image Security Features - Product Requirements

**Feature Request Issue:** #93
**Priority:** High
**Category:** Security / Supply Chain Integrity
**Date:** 2026-02-05

---

## Executive Summary

This document outlines the product requirements for adding **optional container image signing, SBOM generation, and attestation capabilities** to Simple Container CLI (`sc`). The feature enables organizations to meet modern software supply chain security requirements (NIST SP 800-218, SLSA, Executive Order 14028) directly within their existing `sc` workflows.

### Business Value

- **Compliance:** Meet NIST SP 800-218, SLSA Level 2+, Executive Order 14028, and CIS Docker Benchmark requirements
- **Market Access:** Enable AWS Marketplace listing and government contract eligibility
- **Security:** Provide cryptographic proof of image authenticity and software composition transparency
- **Efficiency:** Integrate security tooling into existing `sc` workflows without custom scripts

---

## Problem Statement

Organizations increasingly require software supply chain security capabilities that are currently missing from Simple Container:

1. **No Cryptographic Signing:** Images cannot be signed to prove authenticity
2. **No SBOM Generation:** No way to generate Software Bill of Materials for vulnerability tracking
3. **No Provenance Attestation:** No proof of build source, materials, or integrity
4. **Manual Workarounds Required:** Users resort to complex bash scripts (e.g., 2,400-line `release-images.sh`)

### User Personas

#### Persona 1: DevSecOps Engineer (Primary)
- **Need:** Implement supply chain security without maintaining custom scripts
- **Pain Point:** Complex bash scripts with limited reusability
- **Goal:** Declarative YAML configuration for signing, SBOM, and attestation

#### Persona 2: Compliance Officer
- **Need:** Evidence for NIST, SLSA, and Executive Order compliance
- **Pain Point:** Manual evidence collection from disparate tools
- **Goal:** Automated compliance reporting and artifact generation

#### Persona 3: Application Developer
- **Need:** Deploy services without understanding security tooling
- **Pain Point:** Complex security requirements slow down development
- **Goal:** Zero-configuration security (enabled by DevOps team via config)

---

## Scope

### In Scope

1. **Image Signing** using Cosign with keyless (OIDC) and key-based signing
2. **SBOM Generation** using Syft with CycloneDX and SPDX formats
3. **SLSA Provenance** attestation generation and attachment
4. **Vulnerability Scanning** integration with Grype and Trivy
5. **YAML Configuration** for declarative security policy
6. **CLI Commands** for manual signing, verification, and SBOM operations
7. **CI/CD Integration** with automatic OIDC configuration detection

### Out of Scope (Future Enhancements)

1. Custom signing providers beyond Cosign/Sigstore
2. SBOM vulnerability remediation workflows
3. Policy enforcement engines (e.g., OPA, Kyverno integration)
4. Real-time vulnerability monitoring dashboards
5. Automated security report generation for compliance audits

### Non-Goals

- Replace existing Docker build/push infrastructure
- Support container runtimes other than Docker
- Implement signing for non-container artifacts (Helm charts, binaries)

---

## Functional Requirements

### FR-1: Image Signing with Cosign

**Description:** Enable cryptographic signing of container images using Cosign (Sigstore).

**Configuration Schema:**
```yaml
# In StackConfigSingleImage or StackConfigCompose
security:
  signing:
    enabled: true                    # Default: false
    provider: sigstore               # Currently only "sigstore" supported
    keyless: true                    # Use OIDC-based keyless signing (default: true)
    # Optional key-based signing:
    # privateKey: ${secret:cosign-private-key}
    # publicKey: ${secret:cosign-public-key}
    verify:
      enabled: true                  # Verify after signing (default: true)
      oidcIssuer: "https://token.actions.githubusercontent.com"
      identityRegexp: "^https://github.com/myorg/.*$"
```

**Acceptance Criteria:**
- AC-1.1: Images are signed automatically after `BuildAndPushImage()` completes
- AC-1.2: Keyless signing works with GitHub Actions OIDC token
- AC-1.3: Key-based signing works with private key from secrets manager
- AC-1.4: Signatures are stored in container registry alongside image
- AC-1.5: Signature verification succeeds for signed images
- AC-1.6: Signing failures do not block deployment (fail-open by default)
- AC-1.7: Signing is skipped when `enabled: false` (no performance impact)

**CLI Commands:**
```bash
sc image sign --image docker.example.com/myapp:v1.0.0
sc image sign --image docker.example.com/myapp:v1.0.0 --key cosign.key
sc image verify --image docker.example.com/myapp:v1.0.0
sc stack sign -s mystack -e production
```

**Dependencies:**
- Cosign v3.0.2+ installed on system
- GitHub Actions: `id-token: write` permission for OIDC
- Container registry must support OCI artifacts (ECR, GCR, Harbor, DockerHub)

---

### FR-2: SBOM Generation with Syft

**Description:** Generate Software Bill of Materials (SBOM) in CycloneDX or SPDX format.

**Configuration Schema:**
```yaml
security:
  sbom:
    enabled: true                    # Default: false
    format: cyclonedx-json           # Options: cyclonedx-json, spdx-json, syft-json
    generator: syft                  # Currently only "syft" supported
    attach:
      enabled: true                  # Attach as in-toto attestation (default: true)
      sign: true                     # Sign the SBOM attestation (default: true)
    output:
      local: ./sbom/                 # Save locally (optional)
      registry: true                 # Push to registry (default: true)
```

**Acceptance Criteria:**
- AC-2.1: SBOM is generated for every image build
- AC-2.2: SBOM includes all OS packages and application dependencies
- AC-2.3: SBOM format matches configured format (CycloneDX JSON default)
- AC-2.4: SBOM is attached as OCI attestation when `attach.enabled: true`
- AC-2.5: SBOM attestation is signed when `attach.sign: true`
- AC-2.6: SBOM is saved locally when `output.local` is specified
- AC-2.7: SBOM generation failures are logged but do not block deployment

**CLI Commands:**
```bash
sc sbom generate --image docker.example.com/myapp:v1.0.0 --format cyclonedx-json
sc sbom attach --image docker.example.com/myapp:v1.0.0 --sbom sbom.json
sc sbom verify --image docker.example.com/myapp:v1.0.0
sc stack sbom -s mystack -e production --output ./sboms/
```

**Dependencies:**
- Syft v1.41.0+ installed on system
- Container registry must support OCI artifacts

---

### FR-3: SLSA Provenance Attestation

**Description:** Generate SLSA v1.0 provenance attestation documenting build materials and process.

**Configuration Schema:**
```yaml
security:
  provenance:
    enabled: true                    # Default: false
    version: "1.0"                   # SLSA provenance version
    builder:
      id: "https://github.com/myorg/myrepo"  # Auto-detected from CI
    metadata:
      includeEnv: false              # Include sanitized env vars (default: false)
      includeMaterials: true         # Include source materials (default: true)
```

**Acceptance Criteria:**
- AC-3.1: Provenance is generated with SLSA v1.0 format
- AC-3.2: Builder ID is auto-detected from GitHub Actions context
- AC-3.3: Git commit SHA and repository are included in materials
- AC-3.4: Provenance is signed using same mechanism as image signing
- AC-3.5: Provenance is attached as OCI attestation
- AC-3.6: Provenance generation is skipped when not in CI environment (graceful degradation)

**CLI Commands:**
```bash
sc provenance attach --image docker.example.com/myapp:v1.0.0 \
  --source-repo github.com/myorg/myrepo \
  --git-sha abc123 \
  --workflow-name "Release"
sc provenance verify --image docker.example.com/myapp:v1.0.0
```

**Dependencies:**
- Cosign v3.0.2+ for signing
- CI/CD environment variables (GitHub Actions, GitLab CI, etc.)

---

### FR-4: Vulnerability Scanning Integration

**Description:** Integrate vulnerability scanning with Grype and Trivy for defense-in-depth.

**Configuration Schema:**
```yaml
security:
  scan:
    enabled: true                    # Default: false
    tools:
      - name: grype                  # Primary scanner
        required: true               # Fail deployment on scanner error
        failOn: critical             # Fail on: critical, high, medium, low (optional)
      - name: trivy                  # Validation scanner
        required: false
    upload:
      defectdojo:
        enabled: false
        url: https://defectdojo.example.com
        apiKey: ${secret:defectdojo-api-key}
```

**Acceptance Criteria:**
- AC-4.1: Images are scanned with configured tools after build
- AC-4.2: Scan results are logged to console
- AC-4.3: Deployment fails when `required: true` and scanner finds vulnerabilities matching `failOn` severity
- AC-4.4: Multiple scanners run in parallel for performance
- AC-4.5: Scan results are uploaded to DefectDojo when configured
- AC-4.6: Scanning failures are logged but deployment continues when `required: false`

**CLI Commands:**
```bash
sc image scan --image docker.example.com/myapp:v1.0.0
sc image scan --image docker.example.com/myapp:v1.0.0 --tools grype,trivy
sc stack scan -s mystack -e production
```

**Dependencies:**
- Grype v0.106.0+ installed on system
- Trivy v0.68.2+ installed on system (optional)
- DefectDojo API access (optional)

---

### FR-5: Integrated Release Workflow

**Description:** Combine signing, SBOM, provenance, and scanning into single release command.

**Configuration Schema:**
```yaml
# Combined configuration example
security:
  signing:
    enabled: true
    keyless: true
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
      - name: trivy
        required: false
```

**CLI Command:**
```bash
sc release create -s mystack -e production --version 2026.1.7
```

**Acceptance Criteria:**
- AC-5.1: Single command executes all enabled security features
- AC-5.2: Features execute in optimal order: build → scan → sign → SBOM → provenance
- AC-5.3: Parallel execution for independent operations (scanning with multiple tools)
- AC-5.4: Release fails fast on critical errors (configurable)
- AC-5.5: Release summary shows all security artifacts created
- AC-5.6: Git tag is created after successful release

---

## Non-Functional Requirements

### NFR-1: Performance

- Image signing: < 10 seconds per image (keyless)
- SBOM generation: < 30 seconds per image
- Vulnerability scanning: < 90 seconds per image
- Parallel operations: Support concurrent signing/SBOM for multiple images

### NFR-2: Reliability

- Retry logic: Exponential backoff for transient failures (network, registry)
- Fail-open: Security features fail-open by default (configurable to fail-closed)
- Graceful degradation: Missing tools log warnings but do not crash

### NFR-3: Security

- Private keys: Must be stored in secrets manager (AWS Secrets Manager, GCP Secret Manager)
- OIDC tokens: Must not be logged or persisted
- SBOM privacy: Local SBOM files excluded from git by default

### NFR-4: Compatibility

- Registries: AWS ECR, Google Container Registry, Docker Hub, Harbor, GitHub Container Registry
- CI/CD: GitHub Actions, GitLab CI, Jenkins, CircleCI
- Operating Systems: Linux, macOS (Windows excluded for Phase 1)

### NFR-5: Usability

- Zero-config default: Security features enabled with sane defaults
- Clear error messages: Human-readable errors with remediation steps
- Documentation: Comprehensive guides with examples for each feature

---

## Compliance Mapping

### NIST SP 800-218 (Secure Software Development Framework)

| SSDF Practice | Requirement | Simple Container Feature |
|---------------|-------------|--------------------------|
| **PW.1.3** | Review code before deploying | FR-4: Vulnerability scanning with Grype/Trivy |
| **PS.1.1** | Generate and maintain SBOMs | FR-2: SBOM generation with Syft |
| **PS.3.1** | Archive and protect build artifacts | FR-1: Signed images + Rekor transparency log |
| **PS.3.2** | Verify integrity before use | FR-1: `sc image verify`, FR-2: `sc sbom verify` |
| **RV.1.1** | Identify known vulnerabilities | FR-4: Dual-toolchain scanning |
| **RV.1.3** | Continuously monitor for vulnerabilities | FR-4: DefectDojo integration |

### SLSA (Supply-chain Levels for Software Artifacts)

| SLSA Level | Requirement | Simple Container Feature |
|------------|-------------|--------------------------|
| **Level 1** | Build process fully scripted | Existing `sc` CLI automation |
| **Level 2** | Version control + signed provenance | FR-3: Provenance attestation |
| **Level 3** | Hardened build platform + non-falsifiable provenance | FR-1: Keyless signing via OIDC |

### Executive Order 14028 (Cybersecurity)

- **Section 4(e)(i)** - SBOM provision: ✅ FR-2
- **Section 4(e)(ii)** - Secure software development practices: ✅ FR-4
- **Section 4(e)(iii)** - Provenance and integrity controls: ✅ FR-1, FR-3

---

## Implementation Phasing

### Phase 1: MVP (Core Signing + SBOM)
**Timeline:** 3-4 weeks
**Features:**
- FR-1: Image signing (keyless only)
- FR-2: SBOM generation (CycloneDX only)
- CLI commands: `sc image sign`, `sc image verify`, `sc sbom generate`
- Configuration: Basic YAML schema

**Success Criteria:**
- Users can sign images with keyless OIDC
- Users can generate SBOM in CycloneDX format
- 90% test coverage for core signing/SBOM logic

### Phase 2: Attestation + Scanning
**Timeline:** 2-3 weeks
**Features:**
- FR-3: SLSA provenance
- FR-4: Vulnerability scanning (Grype only)
- CLI commands: `sc provenance attach`, `sc image scan`

**Success Criteria:**
- Provenance attestations pass SLSA verification
- Vulnerability scan blocks deployment on critical CVEs

### Phase 3: Integration + Polish
**Timeline:** 2 weeks
**Features:**
- FR-5: Integrated release workflow
- Key-based signing support
- Multiple SBOM formats (SPDX)
- Trivy scanning integration
- DefectDojo upload

**Success Criteria:**
- Single `sc release create` command executes full workflow
- Performance: < 5 minutes for 9-service release

---

## Risks and Mitigations

### Risk 1: External Tool Dependency
**Description:** Cosign, Syft, Grype versions may break compatibility
**Impact:** High
**Mitigation:**
- Pin tested tool versions in documentation
- Graceful error handling for version mismatches
- Fallback to warning-only mode if tools unavailable

### Risk 2: Registry Compatibility
**Description:** Not all registries support OCI artifacts (attestations)
**Impact:** Medium
**Mitigation:**
- Test with major registries (ECR, GCR, DockerHub, Harbor)
- Document registry requirements
- Local-only SBOM storage as fallback

### Risk 3: OIDC Token Availability
**Description:** Keyless signing requires CI/CD OIDC tokens
**Impact:** Medium
**Mitigation:**
- Support key-based signing as alternative
- Auto-detect CI environment and configure OIDC
- Clear error messages when OIDC unavailable

### Risk 4: Performance Overhead
**Description:** Security operations add 2-5 minutes per image
**Impact:** Low
**Mitigation:**
- Parallelize independent operations
- Make all features opt-in
- Cache SBOM/scan results when image unchanged

---

## Success Metrics

### Adoption Metrics
- **Target:** 20% of users enable signing within 3 months of release
- **Target:** 50% of users enable SBOM generation within 6 months

### Performance Metrics
- **Target:** < 10% overhead for release workflow with all features enabled
- **Target:** Zero performance impact when features disabled

### Quality Metrics
- **Target:** < 5% failure rate for signing operations
- **Target:** 95% test coverage for security package

### Compliance Metrics
- **Target:** 100% NIST SP 800-218 SSDF practices covered
- **Target:** SLSA Level 3 achievable with keyless signing

---

## Open Questions for Architect

1. **Configuration Inheritance:** Should security config inherit from parent stacks?
2. **Error Handling:** Fail-open vs fail-closed default for security features?
3. **Tool Installation:** Should `sc` auto-install Cosign/Syft or require manual installation?
4. **Caching:** How to cache SBOM/scan results to avoid re-scanning unchanged images?
5. **CLI vs Config:** Should security features be CLI-first or config-first?

---

## References

- **NIST SP 800-218:** https://csrc.nist.gov/publications/detail/sp/800-218/final
- **SLSA Framework:** https://slsa.dev/
- **Cosign Documentation:** https://docs.sigstore.dev/cosign/overview/
- **Syft Documentation:** https://github.com/anchore/syft
- **Executive Order 14028:** https://www.whitehouse.gov/briefing-room/presidential-actions/2021/05/12/executive-order-on-improving-the-nations-cybersecurity/
