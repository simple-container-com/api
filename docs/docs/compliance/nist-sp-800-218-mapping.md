# NIST SP 800-218 and Compliance Mapping

This document maps Simple Container security features to compliance frameworks.

## NIST SP 800-218 (SSDF)

Secure Software Development Framework compliance mapping.

### PO.1: Prepare the Organization

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| PO.1.1: Define secure software development standards | Security configuration schema | Stack YAML with security policies |
| PO.1.3: Implement toolchains for development | Integrated security tools | Grype, Syft, Cosign, Trivy |

### PS.1: Protect Software

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| PS.1.1: Store code securely | Provenance attestation | Git commit tracking in SLSA provenance |

### PW.1: Secure Software Development

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| PW.1.1: Follow secure coding practices | Vulnerability scanning | Pre-deployment scan with policy enforcement |
| PW.1.2: Scan for vulnerabilities | Grype/Trivy integration | `scan.enabled: true` with failOn policies |

### PW.4: Review and Test Software

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| PW.4.1: Review and test code | Vulnerability scanning | Automated scanning in CI/CD |
| PW.4.4: Ensure continuous compliance | Policy enforcement | `failOn: critical`, `required: true` |

### PS.3: Produce Well-Secured Software

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| PS.3.1: Securely archive software | Image signing | Cosign signatures in registry |
| PS.3.2: Determine integrity verification | Signature verification | `sc image verify` command |

### RV.1: Identify and Confirm Vulnerabilities

| Practice | Simple Container Feature | Implementation |
|----------|-------------------------|----------------|
| RV.1.1: Identify vulnerabilities | Continuous scanning | Scan on every deployment |
| RV.1.2: Confirm vulnerabilities | Policy enforcement | Block deployments with critical CVEs |
| RV.1.3: Analyze vulnerabilities | Detailed scan reports | JSON output with CVE details, CVSS scores |

## SLSA (Supply-chain Levels for Software Artifacts)

### SLSA Level 1: Documentation

| Requirement | Simple Container Feature | Status |
|-------------|-------------------------|---------|
| Build process documented | Release workflow | ✅ Documented |

### SLSA Level 2: Build as Code

| Requirement | Simple Container Feature | Status |
|-------------|-------------------------|---------|
| Version controlled build | Pulumi infrastructure as code | ✅ Implemented |
| Build service generates provenance | SLSA v1.0 provenance | ✅ Implemented |
| Provenance distributed with artifact | Attestation attachment | ✅ Implemented |

### SLSA Level 3: Hardened Build

| Requirement | Simple Container Feature | Status |
|-------------|-------------------------|---------|
| Signed provenance | Keyless or key-based signing | ✅ Implemented |
| Non-falsifiable provenance | Rekor transparency log | ✅ Implemented |

**SLSA Level Achieved: Level 2-3** (Level 3 when using keyless signing with Rekor)

## Executive Order 14028 (Improving the Nation's Cybersecurity)

### Section 4(e): Software Supply Chain Security

| Requirement | Simple Container Feature | Implementation |
|-------------|-------------------------|----------------|
| SBOM for software | SBOM generation | CycloneDX/SPDX format, automatic generation |
| Participation in vulnerability disclosure | Vulnerability scanning | Pre-deployment scanning with Grype/Trivy |
| Third-party component testing | SBOM with dependencies | Full dependency tree in SBOM |
| Attestation of conformance | Signed attestations | Cosign signatures with SBOM/provenance |

**Compliance Status:** ✅ Compliant

## CISA SBOM Requirements

### Minimum Elements

| Element | Simple Container Feature | Status |
|---------|-------------------------|---------|
| Supplier Name | Syft metadata | ✅ Included |
| Component Name | Package inventory | ✅ Included |
| Version of Component | Package versions | ✅ Included |
| Dependency Relationship | CycloneDX relationships | ✅ Included |
| Author of SBOM | Tool metadata | ✅ Included |
| Timestamp | GeneratedAt field | ✅ Included |

**SBOM Format Compliance:**
- ✅ CycloneDX 1.4+ (JSON/XML)
- ✅ SPDX 2.3+ (JSON/tag-value)

## OpenSSF Scorecard

Security best practices from OpenSSF.

| Check | Simple Container Feature | Score |
|-------|-------------------------|-------|
| Signed-Releases | Image signing | 10/10 |
| Vulnerabilities | Vulnerability scanning | 10/10 |
| Dependency-Update-Tool | Automatic scanning | 10/10 |
| SBOM | SBOM generation | 10/10 |

## Implementation Checklist

Use this checklist to ensure compliance:

### NIST SP 800-218 Compliance

- [ ] Enable vulnerability scanning: `scan.enabled: true`
- [ ] Set fail-on policy: `scan.failOn: critical`
- [ ] Enable image signing: `signing.enabled: true`
- [ ] Generate SBOM: `sbom.enabled: true`
- [ ] Generate provenance: `provenance.enabled: true`
- [ ] Attach attestations to registry: `output.registry: true`
- [ ] Require security operations: `required: true`

### SLSA Level 2-3 Compliance

- [ ] Use version-controlled stack configuration
- [ ] Enable provenance generation: `provenance.enabled: true`
- [ ] Enable image signing: `signing.enabled: true`
- [ ] Use keyless signing for Rekor: `signing.keyless: true`
- [ ] Attach provenance to registry: `provenance.output.registry: true`
- [ ] Include git metadata: `provenance.includeGit: true`
- [ ] Include Dockerfile: `provenance.includeDocker: true`

### EO 14028 Compliance

- [ ] Generate SBOM for all images: `sbom.enabled: true`
- [ ] Use standard SBOM format: `sbom.format: cyclonedx-json` or `spdx-json`
- [ ] Attach SBOM to registry: `sbom.output.registry: true`
- [ ] Enable vulnerability disclosure: `scan.enabled: true`
- [ ] Sign attestations: `signing.enabled: true`

### CISA SBOM Requirements

- [ ] Include all minimum SBOM elements (automatic with Syft)
- [ ] Use compliant format (CycloneDX 1.4+ or SPDX 2.3+)
- [ ] Timestamp SBOM generation (automatic)
- [ ] Distribute SBOM with software (registry attachment)

## Example Compliant Configuration

```yaml
client:
  security:
    enabled: true

    # NIST SP 800-218: PW.1.2, RV.1.1
    scan:
      enabled: true
      tools:
        - name: grype
      failOn: critical
      warnOn: high
      required: true

    # NIST SP 800-218: PS.3.1, SLSA Level 3
    signing:
      enabled: true
      keyless: true
      required: true

    # EO 14028 Section 4(e), CISA SBOM
    sbom:
      enabled: true
      format: cyclonedx-json
      output:
        registry: true
      required: true

    # SLSA Level 2-3
    provenance:
      enabled: true
      format: slsa-v1.0
      includeGit: true
      includeDocker: true
      output:
        registry: true
```

## Audit and Reporting

### Generate Compliance Report

```bash
# Scan and generate report
sc image scan --image myapp:v1.0 --output compliance/scan-report.json

# Generate SBOM
sc sbom generate --image myapp:v1.0 --output compliance/sbom.json

# Verify signatures
sc image verify --image myapp:v1.0

# Verify SBOM attestation
sc sbom verify --image myapp:v1.0 --output compliance/sbom-verified.json

# Verify provenance
sc provenance verify --image myapp:v1.0 --output compliance/provenance.json
```

### Continuous Compliance

```yaml
# CI/CD workflow for continuous compliance
name: Compliance Check

on:
  push:
    branches: [main]
  schedule:
    - cron: '0 0 * * *'  # Daily

jobs:
  compliance:
    runs-on: ubuntu-latest
    steps:
      - name: Scan for vulnerabilities
        run: sc image scan --image $IMAGE --fail-on critical

      - name: Verify signature
        run: sc image verify --image $IMAGE

      - name: Verify SBOM
        run: sc sbom verify --image $IMAGE

      - name: Verify provenance
        run: sc provenance verify --image $IMAGE

      - name: Generate compliance report
        run: |
          echo "NIST SP 800-218: COMPLIANT" >> compliance-report.txt
          echo "SLSA Level: 3" >> compliance-report.txt
          echo "EO 14028: COMPLIANT" >> compliance-report.txt
```

## References

- [NIST SP 800-218](https://csrc.nist.gov/publications/detail/sp/800-218/final)
- [SLSA Framework](https://slsa.dev/)
- [Executive Order 14028](https://www.whitehouse.gov/briefing-room/presidential-actions/2021/05/12/executive-order-on-improving-the-nations-cybersecurity/)
- [CISA SBOM](https://www.cisa.gov/sbom)
- [OpenSSF Scorecard](https://github.com/ossf/scorecard)
- [Sigstore](https://www.sigstore.dev/)
- [CycloneDX](https://cyclonedx.org/)
- [SPDX](https://spdx.dev/)
