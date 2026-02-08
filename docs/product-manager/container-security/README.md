# Container Image Security Features - Product Requirements Summary

**Issue:** #93 - Feature Request: Container Image Signing, SBOM, and Attestation
**Priority:** High
**Status:** Requirements Complete - Ready for Architecture Design
**Date:** 2026-02-05

---

## Quick Links

- **[Full Requirements](./requirements.md)** - Comprehensive product requirements document
- **[Acceptance Criteria](./acceptance-criteria.md)** - Detailed test cases and verification criteria
- **[Task Breakdown](./task-breakdown.md)** - Implementation tasks with effort estimates

---

## Executive Summary

This feature request adds **optional container image signing, SBOM generation, and attestation capabilities** to Simple Container CLI, enabling organizations to meet modern software supply chain security requirements (NIST SP 800-218, SLSA, Executive Order 14028).

### Business Value

- **Compliance:** Meet federal and enterprise security requirements
- **Market Access:** Enable AWS Marketplace listing and government contracts
- **Security:** Cryptographic proof of image authenticity
- **Efficiency:** Integrate security tooling into existing workflows

---

## Scope Overview

### Core Features

1. **Image Signing** - Cosign integration with keyless (OIDC) and key-based signing
2. **SBOM Generation** - Syft integration with CycloneDX and SPDX formats
3. **SLSA Provenance** - Automated provenance attestation
4. **Vulnerability Scanning** - Grype and Trivy integration with DefectDojo upload

### Configuration Example

```yaml
# Optional security configuration in stack YAML
security:
  signing:
    enabled: true
    keyless: true  # Use OIDC for keyless signing
    verify:
      enabled: true
      oidcIssuer: "https://token.actions.githubusercontent.com"

  sbom:
    enabled: true
    format: cyclonedx-json
    attach: true  # Attach as signed attestation

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

### CLI Commands

```bash
# Manual operations
sc image sign --image docker.example.com/myapp:1.0.0
sc image verify --image docker.example.com/myapp:1.0.0
sc sbom generate --image docker.example.com/myapp:1.0.0
sc image scan --image docker.example.com/myapp:1.0.0

# Integrated release workflow
sc release create -s mystack -e production --version 2026.1.7
```

---

## Compliance Coverage

### NIST SP 800-218 SSDF

| Practice | Requirement | Feature |
|----------|-------------|---------|
| PW.1.3 | Review code before deploying | Vulnerability scanning |
| PS.1.1 | Generate and maintain SBOMs | SBOM generation |
| PS.3.1 | Archive and protect artifacts | Signed images + Rekor log |
| PS.3.2 | Verify integrity before use | Image verification |
| RV.1.1 | Identify vulnerabilities | Dual-toolchain scanning |
| RV.1.3 | Continuously monitor | DefectDojo integration |

### SLSA Framework

- **Level 1:** ✅ Fully scripted build process (existing `sc` CLI)
- **Level 2:** ✅ Version control + signed provenance (new feature)
- **Level 3:** ✅ Hardened platform + non-falsifiable provenance (keyless signing)

### Executive Order 14028

- ✅ SBOM provision (Section 4(e)(i))
- ✅ Secure development practices (Section 4(e)(ii))
- ✅ Provenance and integrity (Section 4(e)(iii))

---

## Implementation Phasing

### Phase 1: MVP (3-4 weeks)
- Image signing (keyless only)
- SBOM generation (CycloneDX only)
- CLI commands for manual operations
- Basic YAML configuration

**Success Criteria:** Users can sign images and generate SBOMs

### Phase 2: Attestation + Scanning (2-3 weeks)
- SLSA provenance attestation
- Vulnerability scanning (Grype)
- Fail-on-critical policy enforcement

**Success Criteria:** Provenance passes SLSA verification, scanning blocks critical CVEs

### Phase 3: Integration + Polish (2 weeks)
- Integrated release workflow (`sc release create`)
- Key-based signing support
- Multiple formats (SPDX)
- Trivy integration
- DefectDojo upload

**Success Criteria:** Full workflow completes in < 5 minutes for 9 services

---

## Key Design Principles

### 1. Opt-In by Default
All security features are **disabled by default** to ensure backward compatibility. Users explicitly enable features via YAML configuration.

### 2. Fail-Open Philosophy
Security operations **fail-open by default** (warn but don't block deployment). Users can configure fail-closed behavior for specific checks.

**Rationale:** Prevents security features from breaking existing workflows while encouraging adoption.

### 3. Minimal Code Changes
Leverage external tools (Cosign, Syft, Grype) rather than reimplementing. Integrate via post-build hooks in existing pipeline.

**Integration Point:** `pkg/clouds/pulumi/docker/build_and_push.go`

### 4. Configuration Inheritance
Security config inherits from parent stacks following existing Simple Container patterns.

### 5. CI/CD Aware
Auto-detect CI environment (GitHub Actions, GitLab CI) and configure OIDC automatically for keyless signing.

---

## Architecture Highlights

### New Package Structure

```
pkg/security/
├── signing/       # Cosign wrapper
├── sbom/          # Syft wrapper
├── provenance/    # SLSA provenance
├── scan/          # Grype/Trivy wrappers
├── config.go      # Security config types
└── executor.go    # Orchestrator
```

### Configuration Schema

```go
// Added to StackConfigSingleImage and ComposeService
type SecurityDescriptor struct {
    Signing    *SigningConfig    `json:"signing,omitempty" yaml:"signing,omitempty"`
    SBOM       *SBOMConfig       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
    Provenance *ProvenanceConfig `json:"provenance,omitempty" yaml:"provenance,omitempty"`
    Scan       *ScanConfig       `json:"scan,omitempty" yaml:"scan,omitempty"`
}
```

### Integration Flow

```
BuildAndPushImage()
  → Image built and pushed
  → executeSecurityOperations()
      → Scan (fail fast on critical)
      → Sign image
      → Generate SBOM
      → Attach SBOM attestation
      → Generate provenance
      → Attach provenance attestation
  → Deployment continues
```

---

## Non-Functional Requirements

### Performance
- **Signing:** < 10 seconds per image
- **SBOM:** < 30 seconds per image
- **Scanning:** < 90 seconds per image
- **Overhead:** < 10% when all features enabled

### Reliability
- **Retry:** 3 attempts with exponential backoff for network errors
- **Graceful Degradation:** Missing tools log warnings, don't crash
- **Fail-Open:** Security failures warn but don't block (configurable)

### Security
- **Keys:** Stored in secrets manager only
- **OIDC Tokens:** Never logged or persisted
- **SBOM Privacy:** Local files excluded from git

### Compatibility
- **Registries:** AWS ECR, GCR, Docker Hub, Harbor, GHCR
- **CI/CD:** GitHub Actions, GitLab CI, Jenkins, CircleCI
- **OS:** Linux, macOS (Windows excluded for Phase 1)

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| External tool compatibility | High | Pin tested versions, graceful error handling |
| Registry OCI support | Medium | Test major registries, document requirements |
| OIDC token availability | Medium | Support key-based signing fallback |
| Performance overhead | Low | Parallelize operations, make opt-in |

---

## Success Metrics

### Adoption
- **Target:** 20% of users enable signing within 3 months
- **Target:** 50% of users enable SBOM within 6 months

### Performance
- **Target:** < 10% overhead with all features enabled
- **Target:** Zero impact when features disabled

### Quality
- **Target:** < 5% failure rate for signing operations
- **Target:** 95% test coverage for security package

### Compliance
- **Target:** 100% NIST SP 800-218 coverage
- **Target:** SLSA Level 3 achievable

---

## Open Questions for Architect

1. **Configuration Inheritance:** Should security config inherit from parent stacks? (Recommendation: Yes, follow existing patterns)

2. **Error Handling:** Fail-open vs fail-closed default? (Recommendation: Fail-open default, configurable per feature)

3. **Tool Installation:** Should `sc` auto-install Cosign/Syft? (Recommendation: No, require manual install, provide clear error messages)

4. **Caching:** How to cache SBOM/scan results for unchanged images? (Recommendation: Use image digest as cache key)

5. **CLI vs Config:** Should features be CLI-first or config-first? (Recommendation: Config-first for automation, CLI for manual ops)

---

## Total Effort Estimate

| Phase | Duration | Engineer-Weeks |
|-------|----------|----------------|
| Phase 1: Core Infrastructure & Signing | 3-4 weeks | 2-3 |
| Phase 2: SBOM Generation | 2-3 weeks | 1.5-2 |
| Phase 3: Provenance & Scanning | 2-3 weeks | 2-2.5 |
| Phase 4: Integrated Workflow | 1 week | 0.5-1 |
| Phase 5: Documentation & Polish | 1 week | 0.5-1 |
| **Total** | **9-12 weeks** | **7-10 engineer-weeks** |

**Team:** 2 backend engineers, 1 DevOps, 1 QA, 1 tech writer

---

## Next Steps - Handoff to Architect

### Architect Responsibilities

1. **Architecture Design**
   - Detailed design for security package structure
   - Integration points with existing codebase
   - Resource dependency handling in Pulumi

2. **Implementation Planning**
   - Identify specific files to modify
   - Design API contracts for security interfaces
   - Plan testing strategy

3. **Technical Decisions**
   - Finalize error handling strategy
   - Design caching mechanism
   - Choose between Pulumi resources vs external commands

4. **Risk Assessment**
   - Evaluate registry compatibility
   - Plan performance optimization
   - Design fallback mechanisms

### Artifacts for Architect

- ✅ **requirements.md** - Full functional and non-functional requirements
- ✅ **acceptance-criteria.md** - Test cases and verification criteria
- ✅ **task-breakdown.md** - Detailed implementation tasks with dependencies
- ✅ **README.md** - This summary document

---

## References

- **GitHub Issue:** https://github.com/simple-container-com/api/issues/93
- **NIST SP 800-218:** https://csrc.nist.gov/publications/detail/sp/800-218/final
- **SLSA Framework:** https://slsa.dev/
- **Cosign Documentation:** https://docs.sigstore.dev/cosign/overview/
- **Syft Documentation:** https://github.com/anchore/syft

---

**Product Manager:** Claude (AI Assistant)
**Date Completed:** 2026-02-05
**Status:** ✅ Ready for Architecture Phase
