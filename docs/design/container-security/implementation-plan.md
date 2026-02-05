# Implementation Plan - Container Image Security

**Issue:** #105 - Container Image Security
**Document:** Implementation Plan and Task Breakdown
**Date:** 2026-02-05

---

## Table of Contents

1. [Implementation Strategy](#implementation-strategy)
2. [Phase Breakdown](#phase-breakdown)
3. [File Modifications](#file-modifications)
4. [New Files to Create](#new-files-to-create)
5. [Testing Strategy](#testing-strategy)
6. [Migration Path](#migration-path)

---

## Implementation Strategy

### Development Approach

**Incremental Implementation:**
- Implement features in phases
- Each phase is independently testable
- Maintain backward compatibility throughout

**Testing Strategy:**
- Unit tests first (TDD approach)
- Integration tests after each phase
- E2E tests at completion

**Risk Mitigation:**
- Feature flags for gradual rollout
- Comprehensive error handling
- Extensive logging for debugging

---

## Phase Breakdown

### Phase 1: Core Infrastructure (Week 1-2)

**Goal:** Establish security package foundation and configuration model

**Tasks:**
1. Create security package structure
2. Implement configuration types
3. Implement ExecutionContext with CI detection
4. Implement tool management and version checking
5. Create cache infrastructure
6. Add unit tests (90%+ coverage)

**Deliverables:**
- `pkg/security/config.go`
- `pkg/security/context.go`
- `pkg/security/executor.go`
- `pkg/security/cache.go`
- `pkg/security/errors.go`
- `pkg/security/tools/`
- `pkg/api/security_config.go`

**Success Criteria:**
- Configuration types parse correctly from YAML
- CI environment detection works for GitHub Actions, GitLab CI
- Tool version checking validates cosign, syft, grype
- Cache stores and retrieves results correctly

---

### Phase 2: Image Signing (Week 2-3)

**Goal:** Implement Cosign-based image signing with keyless and key-based modes

**Tasks:**
1. Implement Signer interface
2. Implement KeylessSigner (OIDC)
3. Implement KeyBasedSigner
4. Implement signature verification
5. Add signing to executor workflow
6. Create CLI commands: `sc image sign`, `sc image verify`
7. Add unit and integration tests

**Deliverables:**
- `pkg/security/signing/signer.go`
- `pkg/security/signing/keyless.go`
- `pkg/security/signing/keybased.go`
- `pkg/security/signing/verifier.go`
- `pkg/security/signing/config.go`
- `pkg/cmd/cmd_image/sign.go`
- `pkg/cmd/cmd_image/verify.go`

**Success Criteria:**
- Keyless signing works in GitHub Actions with OIDC
- Key-based signing works with private key from secrets
- Signatures are stored in registry
- Verification succeeds for signed images
- CLI commands functional

---

### Phase 3: SBOM Generation (Week 3-4)

**Goal:** Implement SBOM generation using Syft with attestation support

**Tasks:**
1. Implement Generator interface
2. Implement SyftGenerator
3. Implement multiple format support (CycloneDX, SPDX)
4. Implement Attacher for attestation
5. Add SBOM generation to executor workflow
6. Create CLI commands: `sc sbom generate`, `sc sbom attach`, `sc sbom verify`
7. Add unit and integration tests

**Deliverables:**
- `pkg/security/sbom/generator.go`
- `pkg/security/sbom/syft.go`
- `pkg/security/sbom/attacher.go`
- `pkg/security/sbom/formats.go`
- `pkg/security/sbom/config.go`
- `pkg/cmd/cmd_sbom/generate.go`
- `pkg/cmd/cmd_sbom/attach.go`
- `pkg/cmd/cmd_sbom/verify.go`

**Success Criteria:**
- SBOM generated in CycloneDX JSON format
- SBOM includes all OS packages and dependencies
- SBOM attached as signed attestation
- SBOM saved locally when configured
- CLI commands functional

---

### Phase 4: Provenance & Scanning (Week 4-5)

**Goal:** Implement SLSA provenance and vulnerability scanning

**Tasks:**

**Provenance:**
1. Implement SLSA v1.0 provenance generator
2. Implement build materials collection
3. Implement builder identification
4. Add provenance to executor workflow
5. Create CLI commands: `sc provenance attach`, `sc provenance verify`

**Scanning:**
6. Implement Scanner interface
7. Implement GrypeScanner
8. Implement TrivyScanner (optional)
9. Implement PolicyEnforcer
10. Add scanning to executor workflow (fail-fast)
11. Create CLI command: `sc image scan`
12. Add unit and integration tests

**Deliverables:**
- `pkg/security/provenance/generator.go`
- `pkg/security/provenance/slsa.go`
- `pkg/security/provenance/materials.go`
- `pkg/security/provenance/builder.go`
- `pkg/security/provenance/config.go`
- `pkg/security/scan/scanner.go`
- `pkg/security/scan/grype.go`
- `pkg/security/scan/trivy.go`
- `pkg/security/scan/policy.go`
- `pkg/security/scan/result.go`
- `pkg/security/scan/config.go`
- `pkg/cmd/cmd_provenance/attach.go`
- `pkg/cmd/cmd_provenance/verify.go`
- `pkg/cmd/cmd_image/scan.go`

**Success Criteria:**
- SLSA v1.0 provenance generated with correct structure
- Builder ID auto-detected from CI
- Git commit SHA included in materials
- Provenance attached as signed attestation
- Grype scan detects vulnerabilities
- Policy enforcement blocks on critical vulnerabilities (when configured)
- CLI commands functional

---

### Phase 5: Integration & Release Workflow (Week 6-7)

**Goal:** Integrate with BuildAndPushImage and create unified release workflow

**Tasks:**
1. Modify `pkg/clouds/pulumi/docker/build_and_push.go`
2. Implement Pulumi Command creation for security operations
3. Create release workflow command: `sc release create`
4. Implement parallel execution optimization
5. Add configuration inheritance support
6. Create end-to-end integration tests
7. Performance profiling and optimization
8. Documentation updates

**Deliverables:**
- Modified `pkg/clouds/pulumi/docker/build_and_push.go`
- `pkg/cmd/cmd_release/create.go`
- Integration tests
- Performance benchmarks
- User documentation

**Success Criteria:**
- Security operations execute after image build/push
- Pulumi DAG correctly orders security commands
- Release workflow completes all operations
- Performance overhead < 10%
- Configuration inheritance works
- E2E tests pass

---

## File Modifications

### Existing Files to Modify

#### 1. `pkg/api/client.go`

**Changes:**
- Add `Security *SecurityDescriptor` field to `StackConfigSingleImage`
- Add `Security *SecurityDescriptor` field to `ComposeService`

```go
// StackConfigSingleImage (add field)
type StackConfigSingleImage struct {
    // ... existing fields ...
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}

// ComposeService (add field)
type ComposeService struct {
    // ... existing fields ...
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}
```

#### 2. `pkg/clouds/pulumi/docker/build_and_push.go`

**Changes:**
- Add security operations execution after image push
- Create Pulumi Commands for security operations
- Add dependencies to return value

```go
func BuildAndPushImage(...) (*ImageOut, error) {
    // ... existing build and push logic ...

    var addOpts []sdk.ResourceOption

    // NEW: Execute security operations if configured
    if hasSecurityConfig(stack) {
        securityOpts, err := executeSecurityOperations(ctx, res, stack, params, deployParams, image)
        if err != nil {
            // Log but continue (fail-open by default)
            params.Log.Warn(ctx.Context(), "Security operations failed: %v", err)
        } else {
            addOpts = append(addOpts, securityOpts...)
        }
    }

    addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{res}))
    return &ImageOut{
        Image:   res,
        AddOpts: addOpts,
    }, nil
}

// NEW: Execute security operations
func executeSecurityOperations(...) ([]sdk.ResourceOption, error) {
    // Implementation
}

// NEW: Check if security configured
func hasSecurityConfig(stack api.Stack) bool {
    // Implementation
}
```

#### 3. `pkg/cmd/root_cmd/root.go`

**Changes:**
- Add new command groups for security operations

```go
func InitCommands(rootCmd *cobra.Command) {
    // ... existing commands ...

    // NEW: Security command groups
    rootCmd.AddCommand(cmd_image.NewImageCommand())
    rootCmd.AddCommand(cmd_sbom.NewSBOMCommand())
    rootCmd.AddCommand(cmd_provenance.NewProvenanceCommand())
    rootCmd.AddCommand(cmd_release.NewReleaseCommand())
}
```

#### 4. `cmd/schema-gen/main.go`

**Changes:**
- Add security types to schema generation

```go
func main() {
    // ... existing schema generation ...

    // NEW: Generate security schemas
    generateSchema(&api.SecurityDescriptor{}, "core", "securitydescriptor")
    generateSchema(&api.SigningConfig{}, "core", "signingconfig")
    generateSchema(&api.SBOMConfig{}, "core", "sbomconfig")
    generateSchema(&api.ProvenanceConfig{}, "core", "provenanceconfig")
    generateSchema(&api.ScanConfig{}, "core", "scanconfig")
}
```

---

## New Files to Create

### Core Package (`pkg/security/`)

1. **config.go** - Core configuration types (imported from api package)
2. **executor.go** - Main security operations orchestrator
3. **context.go** - Execution context with CI detection
4. **errors.go** - Security-specific error types
5. **cache.go** - Result caching implementation

### Signing Package (`pkg/security/signing/`)

6. **signer.go** - Signer interface definition
7. **keyless.go** - OIDC keyless signing implementation
8. **keybased.go** - Key-based signing implementation
9. **verifier.go** - Signature verification implementation
10. **config.go** - Signing configuration types

### SBOM Package (`pkg/security/sbom/`)

11. **generator.go** - SBOM generator interface
12. **syft.go** - Syft implementation
13. **attacher.go** - Attestation attachment
14. **formats.go** - Format handling (CycloneDX, SPDX)
15. **config.go** - SBOM configuration types

### Provenance Package (`pkg/security/provenance/`)

16. **generator.go** - Provenance generator interface
17. **slsa.go** - SLSA v1.0 implementation
18. **materials.go** - Build materials collection
19. **builder.go** - Builder identification
20. **config.go** - Provenance configuration types

### Scanning Package (`pkg/security/scan/`)

21. **scanner.go** - Scanner interface definition
22. **grype.go** - Grype implementation
23. **trivy.go** - Trivy implementation
24. **policy.go** - Vulnerability policy enforcement
25. **result.go** - Scan result types
26. **config.go** - Scanner configuration types

### Tools Package (`pkg/security/tools/`)

27. **installer.go** - Tool installation check
28. **command.go** - Command execution wrapper
29. **version.go** - Version compatibility check
30. **registry.go** - Tool registry and metadata

### API Types (`pkg/api/`)

31. **security_config.go** - SecurityDescriptor and related types

### CLI Commands (`pkg/cmd/`)

32. **cmd_image/sign.go** - Image signing command
33. **cmd_image/verify.go** - Signature verification command
34. **cmd_image/scan.go** - Image scanning command
35. **cmd_image/image.go** - Image command group

36. **cmd_sbom/generate.go** - SBOM generation command
37. **cmd_sbom/attach.go** - SBOM attestation command
38. **cmd_sbom/verify.go** - SBOM verification command
39. **cmd_sbom/sbom.go** - SBOM command group

40. **cmd_provenance/attach.go** - Provenance attestation command
41. **cmd_provenance/verify.go** - Provenance verification command
42. **cmd_provenance/provenance.go** - Provenance command group

43. **cmd_release/create.go** - Release workflow command
44. **cmd_release/release.go** - Release command group

### Tests

45. **pkg/security/executor_test.go**
46. **pkg/security/context_test.go**
47. **pkg/security/signing/keyless_test.go**
48. **pkg/security/signing/keybased_test.go**
49. **pkg/security/sbom/syft_test.go**
50. **pkg/security/provenance/slsa_test.go**
51. **pkg/security/scan/grype_test.go**
52. **pkg/security/scan/policy_test.go**
53. **pkg/security/tools/installer_test.go**
54. **pkg/security/integration_test.go** - E2E integration tests

---

## Testing Strategy

### Unit Tests (90%+ Coverage)

**Target Packages:**
- `pkg/security/` - Core executor and context
- `pkg/security/signing/` - All signing implementations
- `pkg/security/sbom/` - SBOM generation and attachment
- `pkg/security/provenance/` - Provenance generation
- `pkg/security/scan/` - Scanning and policy enforcement
- `pkg/security/tools/` - Tool management

**Mocking Strategy:**
- Mock external command execution (cosign, syft, grype)
- Mock CI environment variables
- Mock registry API calls
- Mock file system operations

**Example Test:**

```go
func TestKeylessSigner_Sign(t *testing.T) {
    // Arrange
    mockExec := &mockCommandExecutor{
        output: []byte("Signature pushed to registry\nRekor entry: https://rekor.sigstore.dev/123"),
    }
    signer := &KeylessSigner{
        logger: logger.NewLogger(),
        tools:  mockExec,
    }

    opts := SignOptions{
        OIDCToken:  "eyJhbGciOi...",
        OIDCIssuer: "https://token.actions.githubusercontent.com",
    }

    ref := ImageReference{
        Registry:   "docker.io",
        Repository: "myorg/myapp",
        Digest:     "sha256:abc123",
    }

    // Act
    result, err := signer.Sign(context.Background(), ref, opts)

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, result)
    assert.Contains(t, result.RekorEntry, "rekor.sigstore.dev")
    assert.Equal(t, "sha256:abc123", result.Digest)

    // Verify command was called correctly
    assert.Contains(t, mockExec.lastCommand, "cosign")
    assert.Contains(t, mockExec.lastCommand, "sign")
    assert.Equal(t, "1", mockExec.lastEnv["COSIGN_EXPERIMENTAL"])
}
```

### Integration Tests

**Test Scenarios:**
1. **Full Workflow Test** - Build → Scan → Sign → SBOM → Provenance
2. **GitHub Actions OIDC Test** - Keyless signing with OIDC token
3. **Key-Based Signing Test** - Signing with private key
4. **Scan Policy Test** - Critical vulnerability blocks deployment
5. **Configuration Inheritance Test** - Parent stack config inheritance

**Test Environment:**
- Use Docker-in-Docker for building test images
- Use local registry for push/pull operations
- Mock Sigstore infrastructure (Fulcio, Rekor)

**Example Integration Test:**

```go
func TestFullSecurityWorkflow(t *testing.T) {
    // Setup
    ctx := context.Background()
    registry := startLocalRegistry(t)
    defer registry.Stop()

    // Build test image
    imageRef := buildTestImage(t, registry)

    // Configure security
    config := &SecurityDescriptor{
        Signing:    &SigningConfig{Enabled: true, Keyless: false},
        SBOM:       &SBOMConfig{Enabled: true},
        Provenance: &ProvenanceConfig{Enabled: true},
        Scan:       &ScanConfig{Enabled: true},
    }

    // Execute
    executor, err := NewExecutor(config, mockContext(), logger.NewLogger())
    require.NoError(t, err)

    result, err := executor.Execute(ctx, imageRef)

    // Assert
    assert.NoError(t, err)
    assert.True(t, result.Signed)
    assert.True(t, result.SBOMGenerated)
    assert.True(t, result.ProvenanceGenerated)
    assert.True(t, result.Scanned)
}
```

### End-to-End Tests

**Test Scenarios:**
1. Deploy to AWS ECS with security enabled
2. Deploy to GCP Cloud Run with security enabled
3. Deploy to Kubernetes with security enabled
4. Release workflow with 9 services

**Test Environment:**
- Real AWS/GCP/Kubernetes environments (staging)
- Real container registries
- Real Sigstore infrastructure

---

## Migration Path

### Backward Compatibility

**Zero Impact by Default:**
- All security features disabled by default
- No configuration changes required for existing users
- No performance impact when disabled

**Configuration Migration:**
- No migration required (new optional fields)
- Existing YAML configurations remain valid

### Gradual Adoption Path

**Step 1: Enable Scanning (Non-Blocking)**
```yaml
security:
  scan:
    enabled: true
    tools:
      - name: grype
        required: false  # Non-blocking
        warnOn: high     # Just warn
```

**Step 2: Enable Signing**
```yaml
security:
  signing:
    enabled: true
    keyless: true
```

**Step 3: Enable SBOM**
```yaml
security:
  sbom:
    enabled: true
    format: cyclonedx-json
```

**Step 4: Harden Policies**
```yaml
security:
  scan:
    tools:
      - name: grype
        required: true
        failOn: critical  # Now blocking
```

### Rollback Strategy

**Disable Feature:**
```yaml
security:
  enabled: false  # Master kill switch
```

**Or remove configuration:**
```yaml
# Remove entire security block
# security: ...
```

---

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Create `pkg/security/` package structure
- [ ] Implement `config.go` types
- [ ] Implement `context.go` with CI detection
- [ ] Implement `executor.go` orchestrator
- [ ] Implement `cache.go` caching
- [ ] Implement `tools/` package
- [ ] Add `pkg/api/security_config.go`
- [ ] Write unit tests (90%+ coverage)
- [ ] Update JSON schema generation

### Phase 2: Image Signing
- [ ] Implement `signing/signer.go` interface
- [ ] Implement `signing/keyless.go` OIDC signing
- [ ] Implement `signing/keybased.go` key-based signing
- [ ] Implement `signing/verifier.go` verification
- [ ] Create CLI command `sc image sign`
- [ ] Create CLI command `sc image verify`
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update documentation

### Phase 3: SBOM Generation
- [ ] Implement `sbom/generator.go` interface
- [ ] Implement `sbom/syft.go` Syft wrapper
- [ ] Implement `sbom/attacher.go` attestation
- [ ] Implement `sbom/formats.go` format handling
- [ ] Create CLI command `sc sbom generate`
- [ ] Create CLI command `sc sbom attach`
- [ ] Create CLI command `sc sbom verify`
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update documentation

### Phase 4: Provenance & Scanning
- [ ] Implement `provenance/generator.go` interface
- [ ] Implement `provenance/slsa.go` SLSA v1.0
- [ ] Implement `provenance/materials.go` materials
- [ ] Implement `scan/scanner.go` interface
- [ ] Implement `scan/grype.go` Grype scanner
- [ ] Implement `scan/trivy.go` Trivy scanner
- [ ] Implement `scan/policy.go` policy enforcement
- [ ] Create CLI command `sc provenance attach`
- [ ] Create CLI command `sc provenance verify`
- [ ] Create CLI command `sc image scan`
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update documentation

### Phase 5: Integration & Release
- [ ] Modify `pkg/clouds/pulumi/docker/build_and_push.go`
- [ ] Implement Pulumi Command creation
- [ ] Create CLI command `sc release create`
- [ ] Implement parallel execution
- [ ] Implement configuration inheritance
- [ ] Write E2E integration tests
- [ ] Performance profiling and optimization
- [ ] Update user documentation
- [ ] Create troubleshooting guide
- [ ] Create compliance mapping documentation

---

## Effort Estimates

| Phase | Duration | Engineer-Weeks | Key Milestones |
|-------|----------|----------------|----------------|
| Phase 1: Core Infrastructure | 2 weeks | 1.5-2 | Security package ready |
| Phase 2: Image Signing | 1 week | 1-1.5 | Signing functional |
| Phase 3: SBOM Generation | 1 week | 1-1.5 | SBOM generation working |
| Phase 4: Provenance & Scanning | 2 weeks | 2-2.5 | All security ops functional |
| Phase 5: Integration & Release | 1 week | 1-1.5 | E2E workflow complete |
| **Total** | **7 weeks** | **6.5-9 engineer-weeks** | Production ready |

**Team Composition:**
- 2 Backend Engineers (Go development)
- 1 DevOps Engineer (CI/CD integration, tool testing)
- 1 QA Engineer (testing, validation)
- 1 Technical Writer (documentation)

---

## Success Criteria

### Functional Requirements
- ✅ All acceptance criteria from issue #105 met
- ✅ 90%+ test coverage for security package
- ✅ All CLI commands functional
- ✅ Configuration schema validated

### Non-Functional Requirements
- ✅ < 10% performance overhead when enabled
- ✅ Zero performance impact when disabled
- ✅ < 5% failure rate for signing operations
- ✅ Graceful degradation when tools missing

### Quality Requirements
- ✅ No breaking changes to existing workflows
- ✅ Comprehensive error messages
- ✅ Complete logging for debugging
- ✅ Documentation complete

### Compliance Requirements
- ✅ NIST SP 800-218 coverage complete
- ✅ SLSA Level 3 achievable
- ✅ Executive Order 14028 requirements met

---

## Summary

This implementation plan provides:

1. **Phase Breakdown** - 5 phases over 7 weeks
2. **File Modifications** - Specific files to modify
3. **New Files** - Complete list of new files to create
4. **Testing Strategy** - Unit, integration, and E2E tests
5. **Migration Path** - Backward-compatible adoption strategy
6. **Success Criteria** - Clear completion criteria

**Key Principles:**
- **Incremental Development** - Each phase independently testable
- **Test-Driven** - Unit tests before implementation
- **Backward Compatible** - Zero impact on existing users
- **Well-Documented** - Comprehensive documentation throughout

**Ready for Development:**
- All design documents complete
- Implementation path clear
- Success criteria defined
- Team structure identified

---

**Status:** ✅ Implementation Plan Complete
**Next Phase:** Developer Implementation (Phase 1)
**Related Documents:** [Architecture Overview](./README.md) | [Component Design](./component-design.md) | [API Contracts](./api-contracts.md) | [Integration & Data Flow](./integration-dataflow.md)
