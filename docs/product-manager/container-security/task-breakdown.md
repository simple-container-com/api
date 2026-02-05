# Container Image Security - Implementation Task Breakdown

**Feature Request Issue:** #93
**Date:** 2026-02-05

---

## Overview

This document provides a detailed task breakdown for implementing container image security features. Tasks are organized by phase and include technical specifications, effort estimates, and dependencies.

---

## Phase 1: Core Infrastructure & Image Signing (MVP)

**Timeline:** 3-4 weeks
**Goal:** Enable basic image signing with keyless OIDC support

### Task 1.1: Security Package Structure

**Description:** Create foundational package structure for security features

**Technical Details:**
```
pkg/security/
├── signing/
│   ├── cosign.go          # Cosign wrapper
│   ├── signer.go          # Interface and implementation
│   └── verify.go          # Signature verification
├── sbom/
│   ├── syft.go            # Syft wrapper
│   └── generator.go       # SBOM generation logic
├── provenance/
│   ├── slsa.go            # SLSA provenance generation
│   └── attestation.go     # Attestation attachment
├── scan/
│   ├── grype.go           # Grype wrapper
│   ├── trivy.go           # Trivy wrapper
│   └── scanner.go         # Scanning orchestration
├── config.go              # Security configuration types
├── executor.go            # Security operations orchestrator
└── errors.go              # Security-specific errors
```

**Acceptance Criteria:**
- Package structure follows existing Simple Container conventions
- All packages have godoc comments
- Basic interfaces defined (Signer, Generator, Scanner)

**Effort:** 2 days
**Dependencies:** None
**Priority:** Critical

---

### Task 1.2: Configuration Schema Extension

**Description:** Extend existing configuration structs to support security options

**Technical Details:**
```go
// In pkg/api/client.go

type SecurityDescriptor struct {
    Signing    *SigningConfig    `json:"signing,omitempty" yaml:"signing,omitempty"`
    SBOM       *SBOMConfig       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
    Provenance *ProvenanceConfig `json:"provenance,omitempty" yaml:"provenance,omitempty"`
    Scan       *ScanConfig       `json:"scan,omitempty" yaml:"scan,omitempty"`
}

type SigningConfig struct {
    Enabled    bool              `json:"enabled" yaml:"enabled"`
    Provider   string            `json:"provider" yaml:"provider"` // "sigstore"
    Keyless    bool              `json:"keyless" yaml:"keyless"`
    PrivateKey string            `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
    PublicKey  string            `json:"publicKey,omitempty" yaml:"publicKey,omitempty"`
    Verify     *VerifyConfig     `json:"verify,omitempty" yaml:"verify,omitempty"`
}

type VerifyConfig struct {
    Enabled        bool   `json:"enabled" yaml:"enabled"`
    OIDCIssuer     string `json:"oidcIssuer,omitempty" yaml:"oidcIssuer,omitempty"`
    IdentityRegexp string `json:"identityRegexp,omitempty" yaml:"identityRegexp,omitempty"`
}

// Add to StackConfigSingleImage
type StackConfigSingleImage struct {
    // ... existing fields ...
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}

// Add to StackConfigCompose services
type ComposeService struct {
    // ... existing fields ...
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}
```

**Integration Points:**
- Modify `pkg/api/client.go` to add SecurityDescriptor
- Update `pkg/api/server.go` to support security config inheritance
- Add validation for security configuration

**Acceptance Criteria:**
- Configuration is backward compatible (nil Security = disabled)
- Config parsing works with YAML and JSON
- Validation rejects invalid configurations
- Config can reference secrets: `${secret:cosign-key}`

**Effort:** 3 days
**Dependencies:** Task 1.1
**Priority:** Critical

---

### Task 1.3: Cosign Integration

**Description:** Implement Cosign wrapper for keyless and key-based signing

**Technical Details:**
```go
// pkg/security/signing/cosign.go

package signing

import (
    "context"
    "github.com/sigstore/cosign/v2/pkg/cosign"
)

type CosignSigner struct {
    keyless    bool
    privateKey string
    publicKey  string
}

func NewCosignSigner(config SigningConfig) (*CosignSigner, error) {
    // Validate Cosign is installed
    if !isCosignInstalled() {
        return nil, ErrCosignNotInstalled
    }
    return &CosignSigner{
        keyless:    config.Keyless,
        privateKey: config.PrivateKey,
        publicKey:  config.PublicKey,
    }, nil
}

func (s *CosignSigner) Sign(ctx context.Context, imageRef string) error {
    if s.keyless {
        return s.signKeyless(ctx, imageRef)
    }
    return s.signWithKey(ctx, imageRef)
}

func (s *CosignSigner) signKeyless(ctx context.Context, imageRef string) error {
    // Execute: cosign sign --yes <imageRef>
    // Cosign will automatically obtain OIDC token from environment
    cmd := exec.CommandContext(ctx, "cosign", "sign", "--yes", imageRef)
    return cmd.Run()
}

func (s *CosignSigner) signWithKey(ctx context.Context, imageRef string) error {
    // Write private key to temp file
    // Execute: cosign sign --key <keyfile> --yes <imageRef>
    cmd := exec.CommandContext(ctx, "cosign", "sign", "--key", keyFile, "--yes", imageRef)
    return cmd.Run()
}

func (s *CosignSigner) Verify(ctx context.Context, imageRef string, opts VerifyOptions) error {
    if s.keyless {
        // cosign verify --certificate-identity-regexp <regexp> --certificate-oidc-issuer <issuer> <imageRef>
        cmd := exec.CommandContext(ctx, "cosign", "verify",
            "--certificate-identity-regexp", opts.IdentityRegexp,
            "--certificate-oidc-issuer", opts.OIDCIssuer,
            imageRef)
        return cmd.Run()
    }
    // cosign verify --key <pubkey> <imageRef>
    cmd := exec.CommandContext(ctx, "cosign", "verify", "--key", s.publicKey, imageRef)
    return cmd.Run()
}
```

**Error Handling:**
- Retry logic: 3 attempts with exponential backoff for network errors
- Fail-open: Return warning (not error) when OIDC token unavailable locally
- Clear error messages: "Cosign not found. Install: https://docs.sigstore.dev/cosign/installation"

**Acceptance Criteria:**
- Keyless signing works in GitHub Actions
- Key-based signing works with secrets manager
- Verification succeeds after signing
- Errors are logged with actionable messages
- Network failures are retried

**Effort:** 5 days
**Dependencies:** Task 1.2
**Priority:** Critical

---

### Task 1.4: Build Pipeline Integration

**Description:** Integrate signing into existing BuildAndPushImage flow

**Technical Details:**
```go
// Modify pkg/clouds/pulumi/docker/build_and_push.go

func BuildAndPushImage(ctx *sdk.Context, stack api.Stack, params pApi.ProvisionParams, deployParams api.StackParams, image Image) (*ImageOut, error) {
    // ... existing build and push logic ...

    // NEW: Security operations post-push
    if err := executeSecurityOperations(ctx.Context(), stack, params, imageFullUrl); err != nil {
        // Log warning but don't fail deployment (fail-open)
        params.Log.Warn(ctx.Context(), "Security operations failed: %v", err)
    }

    return &ImageOut{
        Image:   res,
        AddOpts: addOpts,
    }, nil
}

func executeSecurityOperations(ctx context.Context, stack api.Stack, params pApi.ProvisionParams, imageRef string) error {
    securityConfig := getSecurityConfig(stack, params)
    if securityConfig == nil {
        return nil // Security not configured
    }

    executor := security.NewExecutor(securityConfig, params.Log)

    // Execute signing
    if securityConfig.Signing != nil && securityConfig.Signing.Enabled {
        if err := executor.Sign(ctx, imageRef); err != nil {
            params.Log.Warn(ctx, "Image signing failed: %v", err)
            // Don't return error - fail open
        } else {
            params.Log.Info(ctx, "✓ Image signed: %s", imageRef)
        }
    }

    return nil
}
```

**Integration Points:**
- Hook into `BuildAndPushImage()` after successful push
- Access security config from stack configuration
- Log security operations to existing logger
- Ensure Pulumi dependencies are handled correctly

**Acceptance Criteria:**
- Signing executes after image push
- Failed signing logs warning but deployment continues
- No changes to existing BuildAndPushImage signature
- Works with all cloud providers (AWS ECR, GCR, etc.)

**Effort:** 3 days
**Dependencies:** Task 1.3
**Priority:** Critical

---

### Task 1.5: CLI Commands for Manual Signing

**Description:** Add CLI commands for manual signing and verification

**Technical Details:**
```go
// Add to cmd/sc/main.go

var imageCmd = &cobra.Command{
    Use:   "image",
    Short: "Image operations (sign, verify, scan)",
}

var imageSignCmd = &cobra.Command{
    Use:   "sign",
    Short: "Sign a container image",
    Long: `Sign a container image using Cosign.

Examples:
  # Sign with keyless (OIDC)
  sc image sign --image docker.example.com/myapp:1.0.0

  # Sign with private key
  sc image sign --image docker.example.com/myapp:1.0.0 --key cosign.key
`,
    RunE: runImageSign,
}

func runImageSign(cmd *cobra.Command, args []string) error {
    imageRef, _ := cmd.Flags().GetString("image")
    keyFile, _ := cmd.Flags().GetString("key")

    config := api.SigningConfig{
        Enabled:  true,
        Provider: "sigstore",
        Keyless:  keyFile == "",
    }
    if keyFile != "" {
        config.PrivateKey = keyFile
    }

    signer, err := signing.NewCosignSigner(config)
    if err != nil {
        return err
    }

    if err := signer.Sign(cmd.Context(), imageRef); err != nil {
        return fmt.Errorf("signing failed: %w", err)
    }

    fmt.Printf("✓ Image signed: %s\n", imageRef)
    return nil
}
```

**New Commands:**
- `sc image sign --image <ref> [--key <keyfile>]`
- `sc image verify --image <ref> [--key <pubkey>]`
- `sc stack sign -s <stack> -e <env>` (sign all images in stack)

**Acceptance Criteria:**
- Commands work with keyless and key-based signing
- Help text is clear with examples
- Errors show actionable messages
- Commands respect existing `--dry-run` flag

**Effort:** 3 days
**Dependencies:** Task 1.3
**Priority:** Medium

---

### Task 1.6: Unit Tests for Signing

**Description:** Comprehensive unit tests for signing functionality

**Test Coverage:**
- Keyless signing success
- Key-based signing success
- Signing failures (network, missing tools)
- Verification success and failure
- Configuration parsing and validation

**Mocking:**
- Mock `exec.Command` to avoid requiring Cosign in tests
- Mock OIDC token retrieval
- Mock registry interactions

**Acceptance Criteria:**
- 90%+ code coverage for `pkg/security/signing/`
- All edge cases tested
- Integration tests with real Cosign (e2e)

**Effort:** 3 days
**Dependencies:** Task 1.3
**Priority:** High

---

## Phase 2: SBOM Generation

**Timeline:** 2-3 weeks
**Goal:** Generate and attach SBOMs for all images

### Task 2.1: Syft Integration

**Description:** Implement Syft wrapper for SBOM generation

**Technical Details:**
```go
// pkg/security/sbom/syft.go

type SyftGenerator struct {
    format string // cyclonedx-json, spdx-json, syft-json
}

func (g *SyftGenerator) Generate(ctx context.Context, imageRef string) (*SBOM, error) {
    // Execute: syft <imageRef> -o <format>
    cmd := exec.CommandContext(ctx, "syft", imageRef, "-o", g.format)
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("syft failed: %w", err)
    }

    sbom := &SBOM{
        Content:   output,
        Format:    g.format,
        ImageRef:  imageRef,
        Timestamp: time.Now(),
    }
    return sbom, nil
}

func (g *SyftGenerator) Attach(ctx context.Context, imageRef string, sbom *SBOM, signConfig *SigningConfig) error {
    // Write SBOM to temp file
    sbomFile := writeTempFile(sbom.Content)
    defer os.Remove(sbomFile)

    // Attach as in-toto attestation
    // cosign attest --predicate <sbomFile> --type cyclonedx <imageRef>
    args := []string{"attest", "--predicate", sbomFile, "--type", "cyclonedx", imageRef}
    if signConfig != nil && signConfig.Keyless {
        args = append(args, "--yes")
    }
    cmd := exec.CommandContext(ctx, "cosign", args...)
    return cmd.Run()
}
```

**Acceptance Criteria:**
- SBOM generation works for all image types
- CycloneDX and SPDX formats supported
- SBOM includes OS packages and app dependencies
- Attestation attachment succeeds

**Effort:** 4 days
**Dependencies:** Task 1.3 (for attestation signing)
**Priority:** High

---

### Task 2.2: SBOM Configuration and Pipeline Integration

**Description:** Add SBOM config and integrate into build pipeline

**Configuration:**
```go
type SBOMConfig struct {
    Enabled   bool              `json:"enabled" yaml:"enabled"`
    Format    string            `json:"format" yaml:"format"` // cyclonedx-json, spdx-json
    Generator string            `json:"generator" yaml:"generator"` // syft
    Attach    *AttachConfig     `json:"attach,omitempty" yaml:"attach,omitempty"`
    Output    *OutputConfig     `json:"output,omitempty" yaml:"output,omitempty"`
}

type AttachConfig struct {
    Enabled bool `json:"enabled" yaml:"enabled"`
    Sign    bool `json:"sign" yaml:"sign"`
}

type OutputConfig struct {
    Local    string `json:"local,omitempty" yaml:"local,omitempty"` // ./sbom/
    Registry bool   `json:"registry" yaml:"registry"`
}
```

**Pipeline Integration:**
```go
// In executeSecurityOperations()

if securityConfig.SBOM != nil && securityConfig.SBOM.Enabled {
    if err := executor.GenerateSBOM(ctx, imageRef); err != nil {
        params.Log.Warn(ctx, "SBOM generation failed: %v", err)
    } else {
        params.Log.Info(ctx, "✓ SBOM generated: %s", imageRef)
    }
}
```

**Acceptance Criteria:**
- SBOM generation integrates with build pipeline
- Local storage works with configured path
- Registry attachment succeeds
- Configuration validation works

**Effort:** 3 days
**Dependencies:** Task 2.1
**Priority:** High

---

### Task 2.3: SBOM CLI Commands

**Description:** Add CLI commands for SBOM operations

**Commands:**
- `sc sbom generate --image <ref> --format <format> [--output <file>]`
- `sc sbom attach --image <ref> --sbom <file>`
- `sc sbom verify --image <ref>`
- `sc stack sbom -s <stack> -e <env> --output <dir>`

**Acceptance Criteria:**
- All commands work as documented
- Format defaults to cyclonedx-json
- Output path defaults to stdout
- Verification checks attestation signature

**Effort:** 2 days
**Dependencies:** Task 2.1
**Priority:** Medium

---

## Phase 3: SLSA Provenance & Vulnerability Scanning

**Timeline:** 2-3 weeks
**Goal:** Add provenance attestation and vulnerability scanning

### Task 3.1: SLSA Provenance Generation

**Description:** Generate SLSA v1.0 provenance attestations

**Technical Details:**
```go
// pkg/security/provenance/slsa.go

type ProvenanceGenerator struct {
    version string // "1.0"
    config  *ProvenanceConfig
}

func (g *ProvenanceGenerator) Generate(ctx context.Context, imageRef string) (*Provenance, error) {
    // Detect CI environment
    ciEnv := detectCIEnvironment()

    // Build SLSA provenance structure
    provenance := &SLSAProvenance{
        Type: "https://slsa.dev/provenance/v1",
        Predicate: SLSAPredicate{
            BuildType: "https://github.com/simple-container-com/api@v1",
            Builder: Builder{
                ID: ciEnv.BuilderID(),
            },
            Invocation: Invocation{
                ConfigSource: ConfigSource{
                    URI:    ciEnv.RepositoryURI(),
                    Digest: map[string]string{"sha1": ciEnv.CommitSHA()},
                },
            },
        },
    }

    if g.config.Metadata.IncludeMaterials {
        provenance.Predicate.Materials = g.collectMaterials(ctx)
    }

    return provenance, nil
}

func detectCIEnvironment() CIEnvironment {
    if os.Getenv("GITHUB_ACTIONS") == "true" {
        return &GitHubActionsEnv{}
    }
    if os.Getenv("GITLAB_CI") == "true" {
        return &GitLabCIEnv{}
    }
    return &LocalEnv{} // Graceful degradation
}
```

**Acceptance Criteria:**
- SLSA v1.0 provenance structure is correct
- Builder ID auto-detected for GitHub Actions, GitLab CI
- Materials include git commit SHA
- Provenance is attached as signed attestation
- Local builds gracefully skip provenance with warning

**Effort:** 5 days
**Dependencies:** Task 1.3
**Priority:** High

---

### Task 3.2: Vulnerability Scanning with Grype

**Description:** Integrate Grype for vulnerability scanning

**Technical Details:**
```go
// pkg/security/scan/grype.go

type GrypeScanner struct {
    failOn string // critical, high, medium, low
}

func (s *GrypeScanner) Scan(ctx context.Context, imageRef string) (*ScanResult, error) {
    // Execute: grype <imageRef> -o json
    cmd := exec.CommandContext(ctx, "grype", imageRef, "-o", "json")
    output, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("grype scan failed: %w", err)
    }

    result := parseScanResult(output)
    return result, nil
}

func (s *GrypeScanner) ShouldFailBuild(result *ScanResult) bool {
    if s.failOn == "" {
        return false
    }

    severityLevels := map[string]int{
        "critical": 4,
        "high": 3,
        "medium": 2,
        "low": 1,
    }

    threshold := severityLevels[s.failOn]
    for _, vuln := range result.Vulnerabilities {
        if severityLevels[vuln.Severity] >= threshold {
            return true
        }
    }
    return false
}
```

**Acceptance Criteria:**
- Grype scanning works for all image types
- Scan results parsed and logged
- `failOn` threshold enforced correctly
- Scan failures are retried

**Effort:** 4 days
**Dependencies:** None
**Priority:** High

---

### Task 3.3: Multi-Scanner Support (Trivy)

**Description:** Add Trivy as secondary scanner for defense-in-depth

**Technical Details:**
- Similar implementation to Grype
- Parallel execution with Grype
- Result aggregation and deduplication

**Acceptance Criteria:**
- Trivy scanner works independently
- Grype and Trivy run in parallel
- Results are aggregated
- Performance: < 1.5x single scanner time

**Effort:** 3 days
**Dependencies:** Task 3.2
**Priority:** Medium

---

### Task 3.4: DefectDojo Integration

**Description:** Upload scan results to DefectDojo

**Technical Details:**
```go
// pkg/security/scan/defectdojo.go

type DefectDojoUploader struct {
    apiURL string
    apiKey string
}

func (u *DefectDojoUploader) Upload(ctx context.Context, result *ScanResult, imageRef string) error {
    // Convert scan result to DefectDojo format
    payload := convertToDefectDojoFormat(result, imageRef)

    // POST to /api/v2/import-scan/
    req, _ := http.NewRequestWithContext(ctx, "POST",
        u.apiURL+"/api/v2/import-scan/",
        bytes.NewReader(payload))
    req.Header.Set("Authorization", "Token "+u.apiKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("defectdojo upload failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 201 {
        return fmt.Errorf("defectdojo returned %d", resp.StatusCode)
    }

    return nil
}
```

**Acceptance Criteria:**
- Scan results uploaded successfully
- API errors are logged with details
- Upload failures don't block deployment (fail-open)

**Effort:** 2 days
**Dependencies:** Task 3.2
**Priority:** Low

---

## Phase 4: Integrated Release Workflow

**Timeline:** 1 week
**Goal:** Combine all security features into single release command

### Task 4.1: Security Operations Orchestrator

**Description:** Orchestrate all security operations in optimal order

**Technical Details:**
```go
// pkg/security/executor.go

type Executor struct {
    config SecurityConfig
    log    logger.Logger
}

func (e *Executor) ExecuteAll(ctx context.Context, imageRef string) (*Summary, error) {
    summary := &Summary{}

    // Phase 1: Scan (fail fast if critical vulnerabilities)
    if e.config.Scan != nil && e.config.Scan.Enabled {
        scanResult, err := e.executeScan(ctx, imageRef)
        if err != nil && e.config.Scan.Required {
            return nil, fmt.Errorf("scan failed: %w", err)
        }
        summary.ScanResult = scanResult
    }

    // Phase 2: Sign image
    if e.config.Signing != nil && e.config.Signing.Enabled {
        if err := e.executeSign(ctx, imageRef); err != nil {
            e.log.Warn("Signing failed: %v", err)
        } else {
            summary.Signed = true
        }
    }

    // Phase 3: Generate and attach SBOM
    if e.config.SBOM != nil && e.config.SBOM.Enabled {
        sbom, err := e.executeSBOM(ctx, imageRef)
        if err != nil {
            e.log.Warn("SBOM generation failed: %v", err)
        } else {
            summary.SBOMGenerated = true
        }
    }

    // Phase 4: Generate and attach provenance
    if e.config.Provenance != nil && e.config.Provenance.Enabled {
        if err := e.executeProvenance(ctx, imageRef); err != nil {
            e.log.Warn("Provenance generation failed: %v", err)
        } else {
            summary.ProvenanceGenerated = true
        }
    }

    return summary, nil
}
```

**Acceptance Criteria:**
- Operations execute in optimal order
- Parallel execution for independent operations
- Failures are handled gracefully
- Summary includes all results

**Effort:** 3 days
**Dependencies:** All previous tasks
**Priority:** High

---

### Task 4.2: Release Command Implementation

**Description:** Implement `sc release create` command

**Commands:**
- `sc release create -s <stack> -e <env> --version <version>`
- Combines: build → scan → sign → SBOM → provenance → git tag

**Acceptance Criteria:**
- Single command executes full workflow
- Git tag created after success
- Release notes include security summary
- Failed releases don't create git tag

**Effort:** 3 days
**Dependencies:** Task 4.1
**Priority:** High

---

## Phase 5: Documentation & Polish

**Timeline:** 1 week
**Goal:** Complete documentation and user experience improvements

### Task 5.1: User Documentation

**Documents to Create:**
- Getting Started Guide
- Configuration Reference
- CLI Command Reference
- Troubleshooting Guide
- Compliance Mapping Guide

**Effort:** 5 days
**Priority:** High

---

### Task 5.2: Error Message Improvements

**Description:** Ensure all error messages are clear and actionable

**Examples:**
```
❌ Bad:  "signing failed: exit status 1"
✅ Good: "Image signing failed: Cosign not found. Install: https://docs.sigstore.dev/cosign/installation"

❌ Bad:  "OIDC token error"
✅ Good: "Keyless signing requires OIDC token. Add 'id-token: write' to GitHub Actions permissions: https://docs.simple-container.com/signing#github-actions"
```

**Effort:** 2 days
**Priority:** Medium

---

## Dependencies Summary

### External Dependencies
- **Cosign:** v3.0.2+ (image signing)
- **Syft:** v1.41.0+ (SBOM generation)
- **Grype:** v0.106.0+ (vulnerability scanning)
- **Trivy:** v0.68.2+ (optional secondary scanner)

### Internal Dependencies
- Existing secrets management system (for key storage)
- Existing logger package
- Existing build pipeline (`BuildAndPushImage`)
- Existing CLI framework (Cobra)

### CI/CD Dependencies
- GitHub Actions: `id-token: write` permission for OIDC
- Container registry: OCI artifact support

---

## Risk Mitigation

### Risk: External Tool Version Incompatibility
**Mitigation:**
- Pin tested versions in documentation
- Graceful error handling for version mismatches
- Version detection and compatibility warnings

### Risk: Registry Compatibility Issues
**Mitigation:**
- Test with all major registries (ECR, GCR, DockerHub, Harbor)
- Document registry requirements clearly
- Provide fallback to local-only SBOM storage

### Risk: Performance Degradation
**Mitigation:**
- Parallelize independent operations
- Make all features opt-in
- Cache results when images unchanged
- Provide performance benchmarks

---

## Testing Strategy

### Unit Tests
- Target: 90%+ coverage for `pkg/security/`
- Mock external tool executions
- Test all error paths

### Integration Tests
- Test with real Cosign, Syft, Grype
- Test with multiple registries
- Test in GitHub Actions environment

### End-to-End Tests
- Full release workflow with all features enabled
- Multi-service stack release
- Performance benchmarking

### Performance Tests
- Baseline: deployment without security features
- With security: should be < 10% overhead
- Parallel execution: should scale linearly

---

## Success Metrics

### Development Metrics
- All tasks completed on schedule
- 90%+ test coverage achieved
- Zero critical bugs in production

### User Metrics
- 20% adoption within 3 months
- < 5% failure rate for security operations
- Positive user feedback on ease of use

### Compliance Metrics
- 100% NIST SP 800-218 coverage
- SLSA Level 3 achievable
- Executive Order 14028 compliant

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

**Team Recommendation:**
- 2 backend engineers (Go)
- 1 DevOps engineer (CI/CD integration)
- 1 QA engineer (testing)
- 1 technical writer (documentation)

---

## Handoff to Architect

**Key Decisions Required:**
1. Should security config inherit from parent stacks?
2. Default fail-open vs fail-closed for security features?
3. Should `sc` auto-install required tools or require manual installation?
4. How to handle SBOM caching for unchanged images?
5. Should CLI commands be prioritized over config-driven automation?

**Architecture Questions:**
1. Where should SecurityExecutor fit in the existing architecture?
2. How to best integrate with Pulumi resource dependencies?
3. Should security operations be Pulumi resources or external commands?
4. How to handle parallel image processing in multi-service stacks?

**Next Steps:**
1. Review and approve this task breakdown
2. Create detailed architecture design
3. Identify code locations for modifications
4. Design integration points with existing systems
