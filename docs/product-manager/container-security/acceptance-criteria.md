# Container Image Security - Acceptance Criteria & Test Cases

**Feature Request Issue:** #93
**Date:** 2026-02-05

---

## Testing Scope

This document defines comprehensive acceptance criteria and test cases for container image security features (signing, SBOM, attestation, scanning).

---

## Feature 1: Image Signing

### AC-1.1: Automatic Signing After Build

**Test Case:** TC-1.1.1 - Happy Path Keyless Signing
```yaml
Given: A stack with security.signing.enabled=true and keyless=true
When: User runs `sc deploy -s mystack -e production`
Then:
  - Image is built and pushed to registry
  - Image is signed using Cosign keyless signing
  - Signature is stored in registry alongside image
  - Deployment succeeds
  - Logs show: "✓ Image signed: docker.example.com/myapp:1.0.0"
```

**Test Case:** TC-1.1.2 - Signing Disabled
```yaml
Given: A stack with security.signing.enabled=false
When: User runs `sc deploy -s mystack -e production`
Then:
  - Image is built and pushed to registry
  - No signing is attempted
  - Deployment succeeds
  - No signing-related logs appear
```

**Test Case:** TC-1.1.3 - Signing Failure (Fail-Open)
```yaml
Given: A stack with security.signing.enabled=true
  And: OIDC token is not available (running locally)
When: User runs `sc deploy -s mystack -e production`
Then:
  - Image is built and pushed to registry
  - Signing fails with warning: "⚠ Image signing failed: OIDC token not available"
  - Deployment continues and succeeds
  - Exit code is 0
```

### AC-1.2: Keyless Signing with GitHub Actions OIDC

**Test Case:** TC-1.2.1 - GitHub Actions OIDC Auto-Detection
```yaml
Given: Running in GitHub Actions with id-token: write permission
  And: security.signing.enabled=true and keyless=true
When: Workflow executes `sc deploy -s mystack -e production`
Then:
  - OIDC token is automatically obtained from GitHub Actions
  - Image is signed with identity: https://github.com/myorg/myrepo/.github/workflows/deploy.yml@refs/heads/main
  - Signature includes Rekor transparency log entry
  - Signature can be verified with: cosign verify --certificate-identity-regexp "^https://github.com/myorg/.*$"
```

**Test Case:** TC-1.2.2 - Missing OIDC Permission
```yaml
Given: Running in GitHub Actions without id-token: write permission
  And: security.signing.enabled=true and keyless=true
When: Workflow executes `sc deploy -s mystack -e production`
Then:
  - Error: "✗ Image signing failed: OIDC token not available. Add 'id-token: write' to workflow permissions."
  - Deployment continues (fail-open)
  - Exit code is 0 (warning only)
```

### AC-1.3: Key-Based Signing

**Test Case:** TC-1.3.1 - Private Key from Secrets Manager
```yaml
Given: A stack with:
  security:
    signing:
      enabled: true
      keyless: false
      privateKey: ${secret:cosign-private-key}
When: User runs `sc deploy -s mystack -e production`
Then:
  - Private key is retrieved from secrets manager
  - Image is signed with private key
  - Signature is verifiable with corresponding public key
```

**Test Case:** TC-1.3.2 - Missing Private Key
```yaml
Given: A stack with keyless=false but privateKey not specified
When: User runs `sc deploy -s mystack -e production`
Then:
  - Error: "✗ Image signing failed: privateKey required when keyless=false"
  - Deployment continues (fail-open)
```

### AC-1.4: Signature Storage in Registry

**Test Case:** TC-1.4.1 - ECR Signature Storage
```yaml
Given: Image pushed to AWS ECR
  And: Image signed with Cosign
When: User queries registry for attestations
Then:
  - Signature is stored as OCI artifact: sha256-<digest>.sig
  - Signature is retrievable with: cosign verify docker.example.com/myapp:1.0.0
```

### AC-1.5: Signature Verification

**Test Case:** TC-1.5.1 - Verify After Signing
```yaml
Given: security.signing.verify.enabled=true
  And: Image successfully signed
When: Signing completes
Then:
  - Automatic verification is performed
  - Logs show: "✓ Signature verified: docker.example.com/myapp:1.0.0"
  - Deployment continues
```

**Test Case:** TC-1.5.2 - Verification Failure
```yaml
Given: security.signing.verify.enabled=true
  And: Image signature is corrupted
When: Verification is attempted
Then:
  - Error: "✗ Signature verification failed"
  - Deployment fails (exit code 1)
```

### AC-1.6: Fail-Open Behavior

**Test Case:** TC-1.6.1 - Network Failure During Signing
```yaml
Given: Network connectivity to Rekor is unavailable
  And: security.signing.enabled=true and keyless=true
When: User runs `sc deploy -s mystack -e production`
Then:
  - Signing fails with: "⚠ Image signing failed: network error"
  - Deployment continues
  - Exit code is 0
```

### AC-1.7: No Performance Impact When Disabled

**Test Case:** TC-1.7.1 - Performance Baseline
```yaml
Given: security.signing.enabled=false
When: User runs `sc deploy -s mystack -e production` 10 times
Then:
  - Average deployment time: T_baseline
  - No signing-related code is executed
```

**Test Case:** TC-1.7.2 - Performance with Signing Enabled
```yaml
Given: security.signing.enabled=true
When: User runs `sc deploy -s mystack -e production` 10 times
Then:
  - Average deployment time: T_with_signing
  - Overhead: (T_with_signing - T_baseline) / T_baseline < 10%
```

---

## Feature 2: SBOM Generation

### AC-2.1: SBOM Generated for Every Image

**Test Case:** TC-2.1.1 - CycloneDX SBOM Generation
```yaml
Given: security.sbom.enabled=true and format=cyclonedx-json
When: Image is built
Then:
  - Syft is invoked: syft docker.example.com/myapp:1.0.0 -o cyclonedx-json
  - SBOM JSON file is generated
  - SBOM includes image digest, timestamp, and tool version
```

**Test Case:** TC-2.1.2 - Multiple Images in Stack
```yaml
Given: Stack with 3 services (frontend, backend, worker)
  And: security.sbom.enabled=true
When: User runs `sc deploy -s mystack -e production`
Then:
  - 3 SBOMs are generated (one per service)
  - SBOMs are generated in parallel
  - Total SBOM generation time < 60 seconds
```

### AC-2.2: SBOM Includes All Dependencies

**Test Case:** TC-2.2.1 - OS Package Detection
```yaml
Given: Image built from ubuntu:22.04 base
  And: security.sbom.enabled=true
When: SBOM is generated
Then:
  - SBOM includes all OS packages (apt packages)
  - SBOM includes package versions
  - SBOM includes package licenses (where available)
```

**Test Case:** TC-2.2.2 - Application Dependencies
```yaml
Given: Node.js application with package.json
  And: security.sbom.enabled=true
When: SBOM is generated
Then:
  - SBOM includes all npm packages from package-lock.json
  - SBOM includes transitive dependencies
  - SBOM includes package versions and licenses
```

### AC-2.3: SBOM Format Selection

**Test Case:** TC-2.3.1 - SPDX JSON Format
```yaml
Given: security.sbom.format=spdx-json
When: SBOM is generated
Then:
  - SBOM is in SPDX 2.3 JSON format
  - SBOM passes SPDX validation: spdx-validator sbom.json
```

**Test Case:** TC-2.3.2 - Invalid Format
```yaml
Given: security.sbom.format=invalid-format
When: User runs `sc deploy -s mystack -e production`
Then:
  - Error: "✗ Invalid SBOM format: invalid-format. Supported: cyclonedx-json, spdx-json, syft-json"
  - Deployment fails (exit code 1)
```

### AC-2.4: SBOM Attached as OCI Attestation

**Test Case:** TC-2.4.1 - Attestation Attachment
```yaml
Given: security.sbom.attach.enabled=true
When: SBOM is generated
Then:
  - SBOM is attached as in-toto attestation
  - Attestation predicate type: https://cyclonedx.org/bom
  - Attestation is retrievable with: cosign verify-attestation docker.example.com/myapp:1.0.0
```

### AC-2.5: SBOM Attestation Signing

**Test Case:** TC-2.5.1 - Signed SBOM Attestation
```yaml
Given: security.sbom.attach.sign=true
  And: security.signing.enabled=true
When: SBOM is attached as attestation
Then:
  - SBOM attestation is signed with same key/OIDC as image
  - Signature is verifiable with: cosign verify-attestation docker.example.com/myapp:1.0.0
```

### AC-2.6: Local SBOM Storage

**Test Case:** TC-2.6.1 - Save SBOM Locally
```yaml
Given: security.sbom.output.local=./sbom/
When: SBOM is generated
Then:
  - SBOM file is saved to: ./sbom/myapp-1.0.0-cyclonedx.json
  - File permissions: 0644
  - ./sbom/ is added to .gitignore if not already present
```

### AC-2.7: SBOM Generation Failures

**Test Case:** TC-2.7.1 - Syft Not Installed
```yaml
Given: Syft is not installed on system
  And: security.sbom.enabled=true
When: User runs `sc deploy -s mystack -e production`
Then:
  - Warning: "⚠ SBOM generation failed: syft not found. Install: https://github.com/anchore/syft"
  - Deployment continues
  - Exit code is 0
```

---

## Feature 3: SLSA Provenance

### AC-3.1: SLSA v1.0 Format

**Test Case:** TC-3.1.1 - Provenance Structure
```yaml
Given: security.provenance.enabled=true
When: Provenance is generated
Then:
  - Provenance follows SLSA v1.0 schema
  - Provenance includes:
    - buildType: "https://github.com/simple-container-com/api@v1"
    - builder.id: "https://github.com/myorg/myrepo/.github/workflows/deploy.yml@refs/heads/main"
    - invocation.configSource.uri: "git+https://github.com/myorg/myrepo@refs/heads/main"
    - invocation.configSource.digest.sha1: "<commit-sha>"
```

### AC-3.2: Builder ID Auto-Detection

**Test Case:** TC-3.2.1 - GitHub Actions Detection
```yaml
Given: Running in GitHub Actions
  And: GITHUB_REPOSITORY=myorg/myrepo
  And: GITHUB_WORKFLOW=Deploy
When: Provenance is generated
Then:
  - builder.id: "https://github.com/myorg/myrepo/.github/workflows/Deploy@refs/heads/main"
```

**Test Case:** TC-3.2.2 - GitLab CI Detection
```yaml
Given: Running in GitLab CI
  And: CI_PROJECT_PATH=myorg/myrepo
  And: CI_PIPELINE_URL=https://gitlab.com/myorg/myrepo/-/pipelines/12345
When: Provenance is generated
Then:
  - builder.id: "https://gitlab.com/myorg/myrepo/-/pipelines/12345"
```

### AC-3.3: Source Materials Inclusion

**Test Case:** TC-3.3.1 - Git Materials
```yaml
Given: security.provenance.metadata.includeMaterials=true
  And: Git repository at commit abc123
When: Provenance is generated
Then:
  - materials array includes:
    - uri: "git+https://github.com/myorg/myrepo@refs/heads/main"
    - digest.sha1: "abc123"
```

### AC-3.4: Provenance Signing

**Test Case:** TC-3.4.1 - Signed Provenance
```yaml
Given: security.provenance.enabled=true
  And: security.signing.enabled=true
When: Provenance is generated
Then:
  - Provenance is signed as in-toto attestation
  - Attestation predicate type: https://slsa.dev/provenance/v1
  - Signature is verifiable with: cosign verify-attestation docker.example.com/myapp:1.0.0 --type slsaprovenance
```

### AC-3.5: Provenance Attachment

**Test Case:** TC-3.5.1 - OCI Attestation
```yaml
Given: security.provenance.enabled=true
When: Provenance is generated
Then:
  - Provenance is attached to image as OCI artifact
  - Attestation is retrievable from registry
```

### AC-3.6: Graceful Degradation Outside CI

**Test Case:** TC-3.6.1 - Local Build
```yaml
Given: Running on local development machine (not CI)
  And: security.provenance.enabled=true
When: User runs `sc deploy -s mystack -e staging`
Then:
  - Warning: "⚠ Provenance generation skipped: not running in CI environment"
  - Deployment continues
  - Exit code is 0
```

---

## Feature 4: Vulnerability Scanning

### AC-4.1: Image Scanning After Build

**Test Case:** TC-4.1.1 - Grype Scan
```yaml
Given: security.scan.enabled=true
  And: security.scan.tools=[{name: grype}]
When: Image is built
Then:
  - Grype scans image: grype docker.example.com/myapp:1.0.0
  - Scan results are logged to console
  - Scan summary shows: "Found 3 critical, 5 high, 12 medium vulnerabilities"
```

### AC-4.2: Scan Results Logging

**Test Case:** TC-4.2.1 - Console Output Format
```yaml
Given: Image scanned with Grype
When: Scan completes
Then:
  - Logs include vulnerability table:
    | CVE           | Severity | Package        | Version | Fixed In |
    |---------------|----------|----------------|---------|----------|
    | CVE-2024-1234 | Critical | openssl        | 1.1.1  | 1.1.1t   |
```

### AC-4.3: Fail on Critical Vulnerabilities

**Test Case:** TC-4.3.1 - Block Deployment
```yaml
Given: security.scan.tools=[{name: grype, required: true, failOn: critical}]
  And: Image has 2 critical vulnerabilities
When: Scan completes
Then:
  - Error: "✗ Deployment blocked: 2 critical vulnerabilities found"
  - Deployment fails
  - Exit code is 1
```

**Test Case:** TC-4.3.2 - Allow Deployment
```yaml
Given: security.scan.tools=[{name: grype, required: false, failOn: critical}]
  And: Image has 2 critical vulnerabilities
When: Scan completes
Then:
  - Warning: "⚠ 2 critical vulnerabilities found"
  - Deployment continues
  - Exit code is 0
```

### AC-4.4: Parallel Scanning

**Test Case:** TC-4.4.1 - Dual-Toolchain Performance
```yaml
Given: security.scan.tools=[{name: grype}, {name: trivy}]
When: Image is scanned
Then:
  - Grype and Trivy run in parallel
  - Total scan time < 1.5x single scanner time
  - Both results are logged
```

### AC-4.5: DefectDojo Upload

**Test Case:** TC-4.5.1 - Upload Scan Results
```yaml
Given: security.scan.upload.defectdojo.enabled=true
  And: DefectDojo API key configured
When: Scan completes
Then:
  - Scan results are uploaded to DefectDojo
  - API call: POST /api/v2/import-scan/
  - Response: 201 Created
  - Log: "✓ Scan results uploaded to DefectDojo"
```

### AC-4.6: Scanning Failures

**Test Case:** TC-4.6.1 - Grype Not Installed
```yaml
Given: Grype not installed
  And: security.scan.tools=[{name: grype, required: false}]
When: Scan is attempted
Then:
  - Warning: "⚠ Grype not found. Install: https://github.com/anchore/grype"
  - Deployment continues
  - Exit code is 0
```

---

## Feature 5: Integrated Release Workflow

### AC-5.1: Single Command Execution

**Test Case:** TC-5.1.1 - Full Security Release
```yaml
Given: All security features enabled
When: User runs `sc release create -s mystack -e production --version 2026.1.7`
Then:
  - Image is built and pushed
  - Image is scanned (Grype + Trivy)
  - Image is signed (Cosign keyless)
  - SBOM is generated and attached
  - Provenance is generated and attached
  - Git tag "2026.1.7" is created
  - Deployment succeeds
```

### AC-5.2: Optimal Execution Order

**Test Case:** TC-5.2.1 - Execution Sequence
```yaml
Given: All security features enabled
When: Release workflow executes
Then:
  - Order: Build → Scan → Sign → SBOM → Provenance
  - Rationale: Fail fast on vulnerabilities before signing
```

### AC-5.3: Parallel Execution

**Test Case:** TC-5.3.1 - Multi-Image Release
```yaml
Given: Stack with 3 services
  And: All security features enabled
When: User runs `sc release create -s mystack -e production`
Then:
  - All 3 images are processed in parallel
  - Total time < 3x single image time
```

### AC-5.4: Fail-Fast on Critical Errors

**Test Case:** TC-5.4.1 - Build Failure
```yaml
Given: Image build fails
When: Release workflow executes
Then:
  - Workflow stops immediately
  - No security operations are attempted
  - Exit code is 1
```

### AC-5.5: Release Summary

**Test Case:** TC-5.5.1 - Summary Output
```yaml
Given: Release completed successfully
Then:
  - Summary is displayed:
    ✓ 3 images built and pushed
    ✓ 3 images scanned (0 critical vulnerabilities)
    ✓ 3 images signed
    ✓ 3 SBOMs generated and attached
    ✓ 3 provenance attestations attached
    ✓ Git tag created: 2026.1.7
```

### AC-5.6: Git Tag Creation

**Test Case:** TC-5.6.1 - Tag After Success
```yaml
Given: Release completed successfully
When: All security operations succeed
Then:
  - Git tag is created: git tag 2026.1.7
  - Tag is pushed to remote: git push origin 2026.1.7
  - Tag message includes release summary
```

---

## Definition of Done

A feature is considered complete when:

1. ✅ All acceptance criteria are met
2. ✅ All test cases pass
3. ✅ Unit tests achieve 90%+ coverage
4. ✅ Integration tests pass on all supported registries
5. ✅ End-to-end tests pass in GitHub Actions
6. ✅ Documentation is complete with examples
7. ✅ Error messages are clear and actionable
8. ✅ Performance benchmarks meet NFR targets
9. ✅ Security review is completed
10. ✅ User acceptance testing (UAT) is passed

---

## Test Environments

### Environment 1: Local Development
- OS: Linux (Ubuntu 22.04) and macOS (Monterey+)
- Registry: Docker Hub
- CI: None (local execution)
- Purpose: Basic functionality testing

### Environment 2: GitHub Actions
- OS: ubuntu-latest
- Registry: AWS ECR
- CI: GitHub Actions with OIDC
- Purpose: Keyless signing and CI integration testing

### Environment 3: Production Staging
- OS: Linux (Ubuntu 22.04)
- Registry: AWS ECR (production mirror)
- CI: GitHub Actions
- Purpose: Pre-production validation

---

## Sign-Off Criteria

### Development Sign-Off
- [ ] All unit tests pass
- [ ] Code review completed
- [ ] Security review completed

### QA Sign-Off
- [ ] All test cases executed
- [ ] No critical bugs open
- [ ] Performance benchmarks met

### Product Sign-Off
- [ ] User documentation complete
- [ ] Release notes drafted
- [ ] Compliance mapping verified

### DevSecOps Sign-Off
- [ ] Security features tested in production-like environment
- [ ] Compliance requirements validated
- [ ] Operational runbooks created
