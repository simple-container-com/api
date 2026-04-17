# Component Design - Container Image Security

**Issue:** #105 - Container Image Security
**Document:** Detailed Component Design
**Date:** 2026-02-05

---

## Table of Contents

1. [Package Structure](#package-structure)
2. [Core Components](#core-components)
3. [Signing Components](#signing-components)
4. [SBOM Components](#sbom-components)
5. [Provenance Components](#provenance-components)
6. [Scanning Components](#scanning-components)
7. [Tool Management](#tool-management)
8. [Configuration Model](#configuration-model)

---

## Package Structure

```
pkg/security/
├── config.go              # Security configuration types
├── executor.go            # Main security operations orchestrator
├── context.go             # Execution context with CI detection
├── errors.go              # Security-specific error types
├── cache.go               # Result caching implementation
│
├── signing/
│   ├── signer.go          # Signer interface
│   ├── keyless.go         # OIDC keyless signing implementation
│   ├── keybased.go        # Key-based signing implementation
│   ├── verifier.go        # Signature verification
│   └── config.go          # Signing configuration types
│
├── sbom/
│   ├── generator.go       # SBOM generator interface
│   ├── syft.go            # Syft implementation
│   ├── attacher.go        # Attestation attachment
│   ├── formats.go         # Format handling (CycloneDX, SPDX)
│   └── config.go          # SBOM configuration types
│
├── provenance/
│   ├── generator.go       # Provenance generator interface
│   ├── slsa.go            # SLSA v1.0 implementation
│   ├── materials.go       # Build materials collection
│   ├── builder.go         # Builder identification
│   └── config.go          # Provenance configuration types
│
├── scan/
│   ├── scanner.go         # Scanner interface
│   ├── grype.go           # Grype implementation
│   ├── trivy.go           # Trivy implementation
│   ├── policy.go          # Vulnerability policy enforcement
│   ├── result.go          # Scan result types
│   └── config.go          # Scanner configuration types
│
└── tools/
    ├── installer.go       # Tool installation check
    ├── command.go         # Command execution wrapper
    ├── version.go         # Version compatibility check
    └── registry.go        # Tool registry and metadata

pkg/api/
├── security_config.go     # SecurityDescriptor types (added)
└── client.go              # StackConfigSingleImage (modified)

pkg/clouds/pulumi/docker/
└── build_and_push.go      # Integration point (modified)

pkg/cmd/
├── cmd_image/             # Image security commands (new)
│   ├── sign.go
│   ├── verify.go
│   └── scan.go
├── cmd_sbom/              # SBOM commands (new)
│   ├── generate.go
│   ├── attach.go
│   └── verify.go
├── cmd_provenance/        # Provenance commands (new)
│   ├── attach.go
│   └── verify.go
└── cmd_release/           # Release workflow (new)
    └── create.go
```

---

## Core Components

### 1. SecurityExecutor (`executor.go`)

**Purpose:** Main orchestrator for security operations

**Responsibilities:**
- Coordinate signing, SBOM, provenance, and scanning
- Manage execution order and dependencies
- Handle errors and implement fail-open/fail-closed logic
- Parallel execution optimization

**Key Methods:**

```go
type SecurityExecutor struct {
    config  *SecurityDescriptor
    context *ExecutionContext
    logger  *logger.Logger
    cache   *Cache
}

// Execute runs all enabled security operations in correct order
func (e *SecurityExecutor) Execute(ctx context.Context, image ImageReference) (*SecurityResult, error)

// ExecuteWithPulumi integrates with Pulumi resource DAG
func (e *SecurityExecutor) ExecuteWithPulumi(
    pulumiCtx *sdk.Context,
    image *docker.Image,
    config *SecurityDescriptor,
) ([]sdk.ResourceOption, error)
```

**Execution Order:**
1. Validate configuration
2. Check tool availability
3. **Scan** (fail-fast if critical vulnerabilities + failOn: critical)
4. **Sign** image
5. **Generate SBOM** (parallel with step 6-7 if possible)
6. **Attach SBOM** attestation
7. **Generate provenance**
8. **Attach provenance** attestation

**Error Handling:**
- Fail-fast: Scanning with `failOn: critical` stops execution
- Fail-open: Other operations log warnings, continue execution
- Fail-closed: Operations with `required: true` stop execution

### 2. ExecutionContext (`context.go`)

**Purpose:** Capture environment information for security operations

**Responsibilities:**
- Detect CI/CD environment (GitHub Actions, GitLab CI, Jenkins)
- Extract OIDC tokens and CI metadata
- Provide build context information
- Manage authentication credentials

**Structure:**

```go
type ExecutionContext struct {
    // CI Detection
    CI          CIProvider      // github-actions, gitlab-ci, jenkins, none
    IsCI        bool

    // OIDC Information
    OIDCToken   string          // For keyless signing
    OIDCIssuer  string          // Token issuer URL

    // Git Information
    Repository  string          // github.com/org/repo
    Branch      string          // main, feature/xyz
    CommitSHA   string          // Full commit SHA
    CommitShort string          // Short commit SHA (7 chars)

    // Build Information
    BuildID     string          // CI build/run ID
    BuildURL    string          // Link to CI build
    Workflow    string          // Workflow/pipeline name
    Actor       string          // User/service account

    // Registry Information
    Registry    string          // docker.io, gcr.io, etc.

    // Environment
    Environment string          // production, staging, development
    ProjectName string          // Simple Container project name
    StackName   string          // Stack name
}

// NewExecutionContext creates context from environment
func NewExecutionContext(stack api.Stack, params api.StackParams) (*ExecutionContext, error)

// DetectCI identifies CI provider and extracts metadata
func (ctx *ExecutionContext) DetectCI() error

// GetOIDCToken retrieves OIDC token for keyless signing
func (ctx *ExecutionContext) GetOIDCToken() (string, error)

// BuilderID generates SLSA builder identifier
func (ctx *ExecutionContext) BuilderID() string
```

**CI Detection Logic:**

```go
// GitHub Actions
if os.Getenv("GITHUB_ACTIONS") == "true" {
    ctx.CI = CIProviderGitHubActions
    ctx.OIDCToken = requestOIDCToken()
    ctx.Repository = os.Getenv("GITHUB_REPOSITORY")
    ctx.CommitSHA = os.Getenv("GITHUB_SHA")
    ctx.BuildID = os.Getenv("GITHUB_RUN_ID")
    ctx.Actor = os.Getenv("GITHUB_ACTOR")
}

// GitLab CI
if os.Getenv("GITLAB_CI") == "true" {
    ctx.CI = CIProviderGitLabCI
    ctx.OIDCToken = os.Getenv("CI_JOB_JWT_V2")
    ctx.Repository = os.Getenv("CI_PROJECT_PATH")
    // ...
}
```

### 3. Cache (`cache.go`)

**Purpose:** Cache security operation results to avoid redundant work

**Responsibilities:**
- Cache SBOM, scan results, signatures by image digest
- Implement TTL-based expiration
- Persist to disk (`~/.simple-container/cache/security/`)

**Structure:**

```go
type Cache struct {
    baseDir string
    ttl     time.Duration
}

type CacheKey struct {
    Operation   string  // "sbom", "scan-grype", "scan-trivy", "signature"
    ImageDigest string  // sha256:abc123...
    ConfigHash  string  // Hash of relevant config
}

// Get retrieves cached result
func (c *Cache) Get(key CacheKey) ([]byte, bool, error)

// Set stores result in cache
func (c *Cache) Set(key CacheKey, data []byte) error

// Invalidate removes cached result
func (c *Cache) Invalidate(key CacheKey) error

// Clean removes expired entries
func (c *Cache) Clean() error
```

**Cache Strategy:**
- **SBOM:** Cache for 24 hours (rebuilds daily)
- **Scan Results:** Cache for 6 hours (vulnerability databases update frequently)
- **Signatures:** No cache (always verify)
- **Provenance:** No cache (unique per build)

---

## Signing Components

### 1. Signer Interface (`signing/signer.go`)

**Purpose:** Abstract interface for image signing implementations

```go
type Signer interface {
    // Sign signs the container image
    Sign(ctx context.Context, ref ImageReference, opts SignOptions) (*SignResult, error)

    // Verify verifies image signature
    Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*VerifyResult, error)

    // GetPublicKey returns the public key (if applicable)
    GetPublicKey() (string, error)
}

type SignOptions struct {
    // Keyless options
    OIDCToken  string
    OIDCIssuer string

    // Key-based options
    PrivateKey string
    Password   string

    // Common options
    Registry   RegistryAuth
    Annotations map[string]string
}

type SignResult struct {
    Digest     string            // Signed image digest
    Signature  string            // Signature string
    Bundle     string            // Signature bundle (for verification)
    RekorEntry string            // Rekor transparency log entry (keyless)
    Metadata   map[string]string // Additional metadata
}
```

### 2. Keyless Signer (`signing/keyless.go`)

**Purpose:** OIDC-based keyless signing using Cosign/Sigstore

**Implementation:**

```go
type KeylessSigner struct {
    logger *logger.Logger
    tools  *tools.CommandExecutor
}

// Sign implements keyless signing
func (s *KeylessSigner) Sign(ctx context.Context, ref ImageReference, opts SignOptions) (*SignResult, error) {
    // 1. Validate OIDC token available
    if opts.OIDCToken == "" {
        return nil, errors.New("OIDC token required for keyless signing")
    }

    // 2. Build cosign command
    cmd := []string{
        "cosign", "sign",
        "--yes",  // Non-interactive
        ref.String(),
    }

    // 3. Set environment for OIDC
    env := map[string]string{
        "COSIGN_EXPERIMENTAL": "1",  // Enable keyless
        "SIGSTORE_ID_TOKEN":   opts.OIDCToken,
    }

    // 4. Execute signing
    output, err := s.tools.Execute(ctx, cmd, env)
    if err != nil {
        return nil, errors.Wrap(err, "cosign sign failed")
    }

    // 5. Parse Rekor entry from output
    rekorEntry := parseRekorEntry(output)

    // 6. Get image digest
    digest := getImageDigest(ctx, ref)

    return &SignResult{
        Digest:     digest,
        RekorEntry: rekorEntry,
        Metadata: map[string]string{
            "method": "keyless",
            "issuer": opts.OIDCIssuer,
        },
    }, nil
}

// Verify implements keyless verification
func (s *KeylessSigner) Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*VerifyResult, error) {
    cmd := []string{
        "cosign", "verify",
        "--certificate-identity-regexp", opts.IdentityRegexp,
        "--certificate-oidc-issuer", opts.OIDCIssuer,
        ref.String(),
    }

    output, err := s.tools.Execute(ctx, cmd, nil)
    if err != nil {
        return &VerifyResult{Valid: false, Error: err.Error()}, nil
    }

    return &VerifyResult{
        Valid:  true,
        Claims: parseVerificationClaims(output),
    }, nil
}
```

### 3. Key-Based Signer (`signing/keybased.go`)

**Purpose:** Traditional key-based signing with private key

**Implementation:**

```go
type KeyBasedSigner struct {
    logger *logger.Logger
    tools  *tools.CommandExecutor
}

// Sign implements key-based signing
func (s *KeyBasedSigner) Sign(ctx context.Context, ref ImageReference, opts SignOptions) (*SignResult, error) {
    // 1. Write private key to temporary file
    keyFile, err := writePrivateKey(opts.PrivateKey)
    if err != nil {
        return nil, err
    }
    defer os.Remove(keyFile)

    // 2. Build cosign command
    cmd := []string{
        "cosign", "sign",
        "--key", keyFile,
        ref.String(),
    }

    // 3. Set password if provided
    env := map[string]string{}
    if opts.Password != "" {
        env["COSIGN_PASSWORD"] = opts.Password
    }

    // 4. Execute signing
    output, err := s.tools.Execute(ctx, cmd, env)
    if err != nil {
        return nil, errors.Wrap(err, "cosign sign failed")
    }

    digest := getImageDigest(ctx, ref)

    return &SignResult{
        Digest: digest,
        Metadata: map[string]string{
            "method": "key-based",
        },
    }, nil
}
```

### 4. Configuration (`signing/config.go`)

```go
type SigningConfig struct {
    Enabled  bool   `json:"enabled" yaml:"enabled"`
    Provider string `json:"provider,omitempty" yaml:"provider,omitempty"` // Default: "sigstore"
    Keyless  bool   `json:"keyless" yaml:"keyless"`                       // Default: true

    // Key-based signing
    PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty"` // Secret reference
    PublicKey  string `json:"publicKey,omitempty" yaml:"publicKey,omitempty"`
    Password   string `json:"password,omitempty" yaml:"password,omitempty"`     // Secret reference

    // Verification
    Verify *VerifyConfig `json:"verify,omitempty" yaml:"verify,omitempty"`
}

type VerifyConfig struct {
    Enabled        bool   `json:"enabled" yaml:"enabled"`                              // Default: true
    OIDCIssuer     string `json:"oidcIssuer,omitempty" yaml:"oidcIssuer,omitempty"`   // Required for keyless
    IdentityRegexp string `json:"identityRegexp,omitempty" yaml:"identityRegexp,omitempty"` // Optional filter
}
```

---

## SBOM Components

### 1. Generator Interface (`sbom/generator.go`)

**Purpose:** Abstract interface for SBOM generation

```go
type Generator interface {
    // Generate creates SBOM for image
    Generate(ctx context.Context, ref ImageReference, opts GenerateOptions) (*SBOM, error)

    // SupportedFormats returns list of supported output formats
    SupportedFormats() []string
}

type GenerateOptions struct {
    Format      string            // cyclonedx-json, spdx-json, syft-json
    OutputPath  string            // Local file path (optional)
    Catalogers  []string          // Specific catalogers to use
    Scope       string            // all-layers, squashed
}

type SBOM struct {
    Format      string    // Format used
    Content     []byte    // Raw SBOM content
    Digest      string    // SBOM content hash
    ImageDigest string    // Image digest
    GeneratedAt time.Time
    Metadata    SBOMMetadata
}

type SBOMMetadata struct {
    ToolName    string
    ToolVersion string
    PackageCount int
}
```

### 2. Syft Generator (`sbom/syft.go`)

**Purpose:** SBOM generation using Syft

**Implementation:**

```go
type SyftGenerator struct {
    logger *logger.Logger
    tools  *tools.CommandExecutor
}

// Generate creates SBOM using Syft
func (g *SyftGenerator) Generate(ctx context.Context, ref ImageReference, opts GenerateOptions) (*SBOM, error) {
    // 1. Validate format
    if !g.isFormatSupported(opts.Format) {
        return nil, fmt.Errorf("unsupported format: %s", opts.Format)
    }

    // 2. Build syft command
    cmd := []string{
        "syft",
        fmt.Sprintf("registry:%s", ref.String()),
        "-o", opts.Format,
    }

    if opts.OutputPath != "" {
        cmd = append(cmd, "--file", opts.OutputPath)
    }

    // 3. Execute SBOM generation
    output, err := g.tools.Execute(ctx, cmd, nil)
    if err != nil {
        return nil, errors.Wrap(err, "syft generation failed")
    }

    // 4. Parse SBOM metadata
    metadata := g.parseMetadata(output, opts.Format)

    return &SBOM{
        Format:      opts.Format,
        Content:     output,
        Digest:      hashSHA256(output),
        ImageDigest: getImageDigest(ctx, ref),
        GeneratedAt: time.Now(),
        Metadata:    metadata,
    }, nil
}

// SupportedFormats returns Syft-supported formats
func (g *SyftGenerator) SupportedFormats() []string {
    return []string{
        "cyclonedx-json",
        "cyclonedx-xml",
        "spdx-json",
        "spdx-tag-value",
        "syft-json",
    }
}
```

### 3. Attestation Attacher (`sbom/attacher.go`)

**Purpose:** Attach SBOM as in-toto attestation to image

**Implementation:**

```go
type Attacher struct {
    logger *logger.Logger
    tools  *tools.CommandExecutor
    signer signing.Signer
}

// Attach attaches SBOM as signed attestation
func (a *Attacher) Attach(ctx context.Context, ref ImageReference, sbom *SBOM, opts AttachOptions) error {
    // 1. Write SBOM to temporary file
    tmpFile, err := writeSBOMFile(sbom)
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile)

    // 2. Build cosign attest command
    cmd := []string{
        "cosign", "attest",
        "--predicate", tmpFile,
        "--type", "cyclonedx",  // or "spdx"
    }

    // 3. Add signing options (keyless or key-based)
    if opts.Sign {
        if opts.Keyless {
            cmd = append(cmd, "--yes")
            // OIDC token via env
        } else {
            cmd = append(cmd, "--key", opts.KeyPath)
        }
    }

    cmd = append(cmd, ref.String())

    // 4. Execute attestation
    _, err = a.tools.Execute(ctx, cmd, opts.Env)
    if err != nil {
        return errors.Wrap(err, "cosign attest failed")
    }

    a.logger.Info(ctx, "SBOM attestation attached to %s", ref.String())
    return nil
}

// Verify verifies SBOM attestation
func (a *Attacher) Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*SBOM, error) {
    cmd := []string{
        "cosign", "verify-attestation",
        "--type", "cyclonedx",
        ref.String(),
    }

    // Add verification options (certificate identity, issuer)
    if opts.OIDCIssuer != "" {
        cmd = append(cmd, "--certificate-oidc-issuer", opts.OIDCIssuer)
    }

    output, err := a.tools.Execute(ctx, cmd, nil)
    if err != nil {
        return nil, errors.Wrap(err, "attestation verification failed")
    }

    // Parse and return SBOM from attestation
    return parseSBOMFromAttestation(output)
}
```

---

## Provenance Components

### 1. SLSA Generator (`provenance/slsa.go`)

**Purpose:** Generate SLSA v1.0 provenance attestation

**Implementation:**

```go
type SLSAGenerator struct {
    logger  *logger.Logger
    context *ExecutionContext
}

// Generate creates SLSA provenance
func (g *SLSAGenerator) Generate(ctx context.Context, ref ImageReference, opts ProvenanceOptions) (*Provenance, error) {
    // 1. Build SLSA provenance structure
    provenance := &SLSAProvenance{
        Type: "https://slsa.dev/provenance/v1",
        Predicate: SLSAPredicate{
            BuildDefinition: g.buildDefinition(opts),
            RunDetails:      g.runDetails(),
        },
    }

    // 2. Collect build materials
    materials, err := g.collectMaterials(ctx, opts)
    if err != nil {
        return nil, err
    }
    provenance.Predicate.BuildDefinition.ResolvedDependencies = materials

    // 3. Add builder information
    provenance.Predicate.Builder = Builder{
        ID:      g.context.BuilderID(),
        Version: g.getBuildPlatformVersion(),
    }

    // 4. Serialize to JSON
    provenanceJSON, err := json.MarshalIndent(provenance, "", "  ")
    if err != nil {
        return nil, err
    }

    return &Provenance{
        Format:      "slsa-v1.0",
        Content:     provenanceJSON,
        ImageDigest: getImageDigest(ctx, ref),
        GeneratedAt: time.Now(),
    }, nil
}

// buildDefinition creates build definition section
func (g *SLSAGenerator) buildDefinition(opts ProvenanceOptions) BuildDefinition {
    return BuildDefinition{
        BuildType: "https://simple-container.com/build/v1",
        ExternalParameters: map[string]interface{}{
            "repository":  g.context.Repository,
            "ref":         g.context.Branch,
            "workflow":    g.context.Workflow,
        },
        InternalParameters: map[string]interface{}{
            "environment": g.context.Environment,
            "stack":       g.context.StackName,
        },
    }
}

// collectMaterials gathers build materials (source code, dependencies)
func (g *SLSAGenerator) collectMaterials(ctx context.Context, opts ProvenanceOptions) ([]Material, error) {
    materials := []Material{
        {
            URI:    fmt.Sprintf("git+%s@%s", g.context.Repository, g.context.CommitSHA),
            Digest: map[string]string{"sha1": g.context.CommitSHA},
        },
    }

    // Add Dockerfile as material if available
    if opts.Dockerfile != "" {
        dockerfileHash, err := hashFile(opts.Dockerfile)
        if err == nil {
            materials = append(materials, Material{
                URI:    fmt.Sprintf("file://%s", opts.Dockerfile),
                Digest: map[string]string{"sha256": dockerfileHash},
            })
        }
    }

    return materials, nil
}

// runDetails captures runtime information
func (g *SLSAGenerator) runDetails() RunDetails {
    return RunDetails{
        Builder: Builder{
            ID:      g.context.BuilderID(),
            Version: map[string]string{"simple-container": getVersion()},
        },
        Metadata: Metadata{
            InvocationID:  g.context.BuildID,
            StartedOn:     time.Now().Format(time.RFC3339),
            FinishedOn:    time.Now().Format(time.RFC3339),
        },
    }
}
```

**SLSA Provenance Structure:**

```go
type SLSAProvenance struct {
    Type      string         `json:"_type"`
    Predicate SLSAPredicate  `json:"predicate"`
    Subject   []Subject      `json:"subject"`
}

type SLSAPredicate struct {
    BuildDefinition BuildDefinition `json:"buildDefinition"`
    RunDetails      RunDetails      `json:"runDetails"`
}

type BuildDefinition struct {
    BuildType              string                 `json:"buildType"`
    ExternalParameters     map[string]interface{} `json:"externalParameters"`
    InternalParameters     map[string]interface{} `json:"internalParameters"`
    ResolvedDependencies   []Material             `json:"resolvedDependencies"`
}

type Material struct {
    URI    string            `json:"uri"`
    Digest map[string]string `json:"digest"`
}
```

---

## Scanning Components

### 1. Scanner Interface (`scan/scanner.go`)

**Purpose:** Abstract interface for vulnerability scanners

```go
type Scanner interface {
    // Scan performs vulnerability scan on image
    Scan(ctx context.Context, ref ImageReference, opts ScanOptions) (*ScanResult, error)

    // Name returns scanner name
    Name() string

    // Version returns scanner version
    Version() (string, error)
}

type ScanOptions struct {
    FailOn     Severity     // critical, high, medium, low
    WarnOn     Severity     // Severity to warn (not fail)
    Scope      string       // all-layers, squashed
    OutputPath string       // Save results to file
}

type ScanResult struct {
    Scanner      string               // grype, trivy
    Version      string               // Scanner version
    ImageDigest  string               // Scanned image digest
    Vulnerabilities []Vulnerability   // Found vulnerabilities
    Summary      VulnerabilitySummary
    ScannedAt    time.Time
}

type Vulnerability struct {
    ID          string   // CVE-2023-1234
    Severity    Severity // critical, high, medium, low
    Package     string   // Package name
    Version     string   // Installed version
    FixedIn     string   // Fixed version (if available)
    Description string   // Vulnerability description
    URLs        []string // Reference URLs
}

type VulnerabilitySummary struct {
    Critical int
    High     int
    Medium   int
    Low      int
    Total    int
}
```

### 2. Grype Scanner (`scan/grype.go`)

**Purpose:** Vulnerability scanning using Grype

**Implementation:**

```go
type GrypeScanner struct {
    logger *logger.Logger
    tools  *tools.CommandExecutor
}

// Scan performs Grype vulnerability scan
func (s *GrypeScanner) Scan(ctx context.Context, ref ImageReference, opts ScanOptions) (*ScanResult, error) {
    // 1. Build grype command
    cmd := []string{
        "grype",
        fmt.Sprintf("registry:%s", ref.String()),
        "-o", "json",  // JSON output for parsing
    }

    if opts.Scope != "" {
        cmd = append(cmd, "--scope", opts.Scope)
    }

    // 2. Execute scan
    output, err := s.tools.Execute(ctx, cmd, nil)
    if err != nil {
        return nil, errors.Wrap(err, "grype scan failed")
    }

    // 3. Parse JSON results
    var grypeOutput GrypeOutput
    if err := json.Unmarshal(output, &grypeOutput); err != nil {
        return nil, errors.Wrap(err, "failed to parse grype output")
    }

    // 4. Convert to standard ScanResult format
    result := s.convertToScanResult(grypeOutput, ref)

    // 5. Apply policy enforcement
    if err := s.enforcePolicy(result, opts); err != nil {
        return result, err  // Return result with error
    }

    return result, nil
}

// enforcePolicy applies vulnerability policy
func (s *GrypeScanner) enforcePolicy(result *ScanResult, opts ScanOptions) error {
    // Check fail-on threshold
    switch opts.FailOn {
    case SeverityCritical:
        if result.Summary.Critical > 0 {
            return fmt.Errorf("found %d critical vulnerabilities (failOn: critical)", result.Summary.Critical)
        }
    case SeverityHigh:
        if result.Summary.Critical > 0 || result.Summary.High > 0 {
            return fmt.Errorf("found %d critical + %d high vulnerabilities (failOn: high)",
                result.Summary.Critical, result.Summary.High)
        }
    }

    // Warnings don't fail, just log
    if opts.WarnOn != "" {
        s.logger.Warn(ctx, "Found vulnerabilities: %d critical, %d high, %d medium, %d low",
            result.Summary.Critical, result.Summary.High, result.Summary.Medium, result.Summary.Low)
    }

    return nil
}
```

### 3. Policy Enforcement (`scan/policy.go`)

**Purpose:** Centralized vulnerability policy enforcement

```go
type PolicyEnforcer struct {
    logger *logger.Logger
}

// Enforce applies policy to scan results
func (p *PolicyEnforcer) Enforce(ctx context.Context, results []*ScanResult, config *ScanConfig) error {
    // Aggregate results from multiple scanners
    aggregated := p.aggregateResults(results)

    // Apply tool-specific policies
    for _, toolConfig := range config.Tools {
        toolResult := findResultByScanner(results, toolConfig.Name)
        if toolResult == nil {
            if toolConfig.Required {
                return fmt.Errorf("required scanner %s did not complete", toolConfig.Name)
            }
            continue
        }

        // Check fail-on threshold
        if err := p.checkThreshold(toolResult, toolConfig.FailOn); err != nil {
            return err
        }
    }

    p.logger.Info(ctx, "Vulnerability policy check passed: %d total vulnerabilities found", aggregated.Summary.Total)
    return nil
}

// aggregateResults combines results from multiple scanners
func (p *PolicyEnforcer) aggregateResults(results []*ScanResult) *ScanResult {
    // Deduplicate vulnerabilities by CVE ID
    vulnMap := make(map[string]Vulnerability)

    for _, result := range results {
        for _, vuln := range result.Vulnerabilities {
            if existing, ok := vulnMap[vuln.ID]; ok {
                // Keep higher severity
                if vuln.Severity > existing.Severity {
                    vulnMap[vuln.ID] = vuln
                }
            } else {
                vulnMap[vuln.ID] = vuln
            }
        }
    }

    // Convert back to slice
    aggregated := &ScanResult{
        Scanner:    "aggregated",
        Vulnerabilities: make([]Vulnerability, 0, len(vulnMap)),
    }

    for _, vuln := range vulnMap {
        aggregated.Vulnerabilities = append(aggregated.Vulnerabilities, vuln)
        switch vuln.Severity {
        case SeverityCritical:
            aggregated.Summary.Critical++
        case SeverityHigh:
            aggregated.Summary.High++
        case SeverityMedium:
            aggregated.Summary.Medium++
        case SeverityLow:
            aggregated.Summary.Low++
        }
    }

    aggregated.Summary.Total = len(aggregated.Vulnerabilities)
    return aggregated
}
```

---

## Tool Management

### 1. Tool Installer (`tools/installer.go`)

**Purpose:** Check for required tool installation and versions

```go
type ToolInstaller struct {
    logger   *logger.Logger
    executor *CommandExecutor
}

type ToolMetadata struct {
    Name            string
    Command         string
    MinVersion      string
    InstallURL      string
    Required        bool
}

// CheckInstalled verifies tool is installed and meets version requirements
func (i *ToolInstaller) CheckInstalled(ctx context.Context, tool ToolMetadata) (bool, string, error) {
    // 1. Check if command exists
    cmd := exec.CommandContext(ctx, "which", tool.Command)
    if err := cmd.Run(); err != nil {
        return false, "", fmt.Errorf("%s not found in PATH", tool.Command)
    }

    // 2. Get version
    version, err := i.getVersion(ctx, tool)
    if err != nil {
        return false, "", err
    }

    // 3. Compare version
    if tool.MinVersion != "" {
        if !i.meetsVersion(version, tool.MinVersion) {
            return false, version, fmt.Errorf("%s version %s does not meet minimum %s",
                tool.Name, version, tool.MinVersion)
        }
    }

    return true, version, nil
}

// getVersion extracts version from tool
func (i *ToolInstaller) getVersion(ctx context.Context, tool ToolMetadata) (string, error) {
    var cmd *exec.Cmd

    switch tool.Name {
    case "cosign":
        cmd = exec.CommandContext(ctx, "cosign", "version")
    case "syft":
        cmd = exec.CommandContext(ctx, "syft", "version")
    case "grype":
        cmd = exec.CommandContext(ctx, "grype", "version")
    case "trivy":
        cmd = exec.CommandContext(ctx, "trivy", "--version")
    default:
        cmd = exec.CommandContext(ctx, tool.Command, "--version")
    }

    output, err := cmd.Output()
    if err != nil {
        return "", err
    }

    return parseVersion(string(output)), nil
}

// CheckAllTools validates all required tools for configuration
func (i *ToolInstaller) CheckAllTools(ctx context.Context, config *SecurityDescriptor) error {
    var missing []string
    var incompatible []string

    // Check signing tools
    if config.Signing != nil && config.Signing.Enabled {
        ok, version, err := i.CheckInstalled(ctx, ToolRegistry["cosign"])
        if !ok {
            missing = append(missing, fmt.Sprintf("cosign: %v", err))
        } else {
            i.logger.Info(ctx, "Found cosign version %s", version)
        }
    }

    // Check SBOM tools
    if config.SBOM != nil && config.SBOM.Enabled {
        ok, version, err := i.CheckInstalled(ctx, ToolRegistry["syft"])
        if !ok {
            missing = append(missing, fmt.Sprintf("syft: %v", err))
        } else {
            i.logger.Info(ctx, "Found syft version %s", version)
        }
    }

    // Check scanning tools
    if config.Scan != nil && config.Scan.Enabled {
        for _, toolConfig := range config.Scan.Tools {
            ok, version, err := i.CheckInstalled(ctx, ToolRegistry[toolConfig.Name])
            if !ok && toolConfig.Required {
                missing = append(missing, fmt.Sprintf("%s: %v", toolConfig.Name, err))
            } else if ok {
                i.logger.Info(ctx, "Found %s version %s", toolConfig.Name, version)
            }
        }
    }

    if len(missing) > 0 {
        return fmt.Errorf("missing required tools:\n%s\n\nSee installation guide: https://docs.simple-container.com/security/tools",
            strings.Join(missing, "\n"))
    }

    return nil
}
```

### 2. Tool Registry (`tools/registry.go`)

```go
var ToolRegistry = map[string]ToolMetadata{
    "cosign": {
        Name:       "Cosign",
        Command:    "cosign",
        MinVersion: "v3.0.2",
        InstallURL: "https://docs.sigstore.dev/cosign/installation/",
        Required:   true,
    },
    "syft": {
        Name:       "Syft",
        Command:    "syft",
        MinVersion: "v1.41.0",
        InstallURL: "https://github.com/anchore/syft#installation",
        Required:   true,
    },
    "grype": {
        Name:       "Grype",
        Command:    "grype",
        MinVersion: "v0.106.0",
        InstallURL: "https://github.com/anchore/grype#installation",
        Required:   true,
    },
    "trivy": {
        Name:       "Trivy",
        Command:    "trivy",
        MinVersion: "v0.68.2",
        InstallURL: "https://aquasecurity.github.io/trivy/latest/getting-started/installation/",
        Required:   false,
    },
}
```

---

## Configuration Model

### SecurityDescriptor (added to `pkg/api/security_config.go`)

```go
// SecurityDescriptor defines security operations for container images
type SecurityDescriptor struct {
    Signing    *SigningConfig    `json:"signing,omitempty" yaml:"signing,omitempty"`
    SBOM       *SBOMConfig       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
    Provenance *ProvenanceConfig `json:"provenance,omitempty" yaml:"provenance,omitempty"`
    Scan       *ScanConfig       `json:"scan,omitempty" yaml:"scan,omitempty"`
}

// SigningConfig configures image signing
type SigningConfig struct {
    Enabled    bool          `json:"enabled" yaml:"enabled"`
    Provider   string        `json:"provider,omitempty" yaml:"provider,omitempty"` // Default: "sigstore"
    Keyless    bool          `json:"keyless" yaml:"keyless"`                       // Default: true
    PrivateKey string        `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
    PublicKey  string        `json:"publicKey,omitempty" yaml:"publicKey,omitempty"`
    Password   string        `json:"password,omitempty" yaml:"password,omitempty"`
    Verify     *VerifyConfig `json:"verify,omitempty" yaml:"verify,omitempty"`
}

type VerifyConfig struct {
    Enabled        bool   `json:"enabled" yaml:"enabled"`
    OIDCIssuer     string `json:"oidcIssuer,omitempty" yaml:"oidcIssuer,omitempty"`
    IdentityRegexp string `json:"identityRegexp,omitempty" yaml:"identityRegexp,omitempty"`
}

// SBOMConfig configures SBOM generation
type SBOMConfig struct {
    Enabled   bool          `json:"enabled" yaml:"enabled"`
    Format    string        `json:"format,omitempty" yaml:"format,omitempty"` // Default: "cyclonedx-json"
    Generator string        `json:"generator,omitempty" yaml:"generator,omitempty"` // Default: "syft"
    Attach    *AttachConfig `json:"attach,omitempty" yaml:"attach,omitempty"`
    Output    *OutputConfig `json:"output,omitempty" yaml:"output,omitempty"`
}

type AttachConfig struct {
    Enabled bool `json:"enabled" yaml:"enabled"` // Default: true
    Sign    bool `json:"sign" yaml:"sign"`       // Default: true
}

type OutputConfig struct {
    Local    string `json:"local,omitempty" yaml:"local,omitempty"`       // Local directory
    Registry bool   `json:"registry" yaml:"registry"`                      // Default: true
}

// ProvenanceConfig configures SLSA provenance
type ProvenanceConfig struct {
    Enabled  bool              `json:"enabled" yaml:"enabled"`
    Version  string            `json:"version,omitempty" yaml:"version,omitempty"` // Default: "1.0"
    Builder  *BuilderConfig    `json:"builder,omitempty" yaml:"builder,omitempty"`
    Metadata *MetadataConfig   `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type BuilderConfig struct {
    ID string `json:"id,omitempty" yaml:"id,omitempty"` // Auto-detected from CI
}

type MetadataConfig struct {
    IncludeEnv       bool `json:"includeEnv" yaml:"includeEnv"`             // Default: false
    IncludeMaterials bool `json:"includeMaterials" yaml:"includeMaterials"` // Default: true
}

// ScanConfig configures vulnerability scanning
type ScanConfig struct {
    Enabled bool             `json:"enabled" yaml:"enabled"`
    Tools   []ScanToolConfig `json:"tools,omitempty" yaml:"tools,omitempty"`
}

type ScanToolConfig struct {
    Name     string   `json:"name" yaml:"name"`               // grype, trivy
    Required bool     `json:"required" yaml:"required"`       // Default: true for grype
    FailOn   Severity `json:"failOn,omitempty" yaml:"failOn,omitempty"` // critical, high, medium, low
    WarnOn   Severity `json:"warnOn,omitempty" yaml:"warnOn,omitempty"`
}

type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityMedium   Severity = "medium"
    SeverityLow      Severity = "low"
)
```

---

## Summary

This component design provides:

1. **Modular Architecture** - Independent packages for signing, SBOM, provenance, scanning
2. **Interface-Based** - Easy to extend with new implementations
3. **Configuration-Driven** - Declarative YAML configuration
4. **CI/CD Aware** - Automatic environment detection
5. **Tool Abstraction** - Wrappers for external tools (Cosign, Syft, Grype, Trivy)
6. **Policy Enforcement** - Flexible fail-open/fail-closed behavior
7. **Caching** - Performance optimization
8. **Error Handling** - Graceful degradation

**Next Steps:**
- Review [API Contracts](./api-contracts.md) for detailed interfaces
- Review [Integration & Data Flow](./integration-dataflow.md) for execution flow
- Review [Implementation Plan](./implementation-plan.md) for development tasks

---

**Status:** ✅ Component Design Complete
**Related Documents:** [Architecture Overview](./README.md) | [API Contracts](./api-contracts.md) | [Integration & Data Flow](./integration-dataflow.md)
