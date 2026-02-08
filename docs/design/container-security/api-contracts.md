# API Contracts - Container Image Security

**Issue:** #105 - Container Image Security
**Document:** API Contracts and Interface Specifications
**Date:** 2026-02-05

---

## Table of Contents

1. [Core Types](#core-types)
2. [Security Executor API](#security-executor-api)
3. [Signing API](#signing-api)
4. [SBOM API](#sbom-api)
5. [Provenance API](#provenance-api)
6. [Scanning API](#scanning-api)
7. [Tool Management API](#tool-management-api)
8. [CLI Commands API](#cli-commands-api)

---

## Core Types

### ImageReference

Represents a container image with registry, repository, and tag/digest information.

```go
// ImageReference represents a container image reference
type ImageReference struct {
    Registry   string // docker.io, gcr.io, 123456789.dkr.ecr.us-east-1.amazonaws.com
    Repository string // myorg/myapp
    Tag        string // v1.0.0, latest
    Digest     string // sha256:abc123... (optional, preferred over tag)
}

// String returns the full image reference
func (r ImageReference) String() string {
    if r.Digest != "" {
        return fmt.Sprintf("%s/%s@%s", r.Registry, r.Repository, r.Digest)
    }
    return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, r.Tag)
}

// WithDigest returns a new reference with digest
func (r ImageReference) WithDigest(digest string) ImageReference {
    return ImageReference{
        Registry:   r.Registry,
        Repository: r.Repository,
        Digest:     digest,
    }
}

// ParseImageReference parses image string into ImageReference
func ParseImageReference(image string) (ImageReference, error)
```

### RegistryAuth

Authentication credentials for container registries.

```go
// RegistryAuth holds registry authentication credentials
type RegistryAuth struct {
    Username string
    Password string
    Token    string // For token-based auth
}

// FromDockerConfig loads credentials from ~/.docker/config.json
func (a *RegistryAuth) FromDockerConfig(registry string) error

// FromEnvironment loads credentials from environment variables
func (a *RegistryAuth) FromEnvironment() error
```

### SecurityResult

Aggregated result from all security operations.

```go
// SecurityResult contains results from all security operations
type SecurityResult struct {
    Image        ImageReference

    // Operation results
    Signed       bool
    SignResult   *SignResult

    SBOMGenerated bool
    SBOM          *SBOM

    ProvenanceGenerated bool
    Provenance          *Provenance

    Scanned      bool
    ScanResults  []*ScanResult

    // Timing
    StartedAt    time.Time
    FinishedAt   time.Time
    Duration     time.Duration

    // Errors
    Errors       []error
    Warnings     []string
}

// HasCriticalIssues returns true if any operation found critical issues
func (r *SecurityResult) HasCriticalIssues() bool

// Summary returns human-readable summary
func (r *SecurityResult) Summary() string
```

---

## Security Executor API

Main orchestrator for security operations.

### Interface

```go
package security

// Executor orchestrates security operations
type Executor interface {
    // Execute runs all enabled security operations
    Execute(ctx context.Context, image ImageReference) (*SecurityResult, error)

    // ExecuteWithPulumi integrates with Pulumi resource DAG
    ExecuteWithPulumi(
        pulumiCtx *sdk.Context,
        dockerImage *docker.Image,
        config *SecurityDescriptor,
    ) ([]sdk.ResourceOption, error)

    // ValidateConfig validates security configuration
    ValidateConfig(config *SecurityDescriptor) error
}

// NewExecutor creates a new security executor
func NewExecutor(
    config *SecurityDescriptor,
    context *ExecutionContext,
    logger *logger.Logger,
) (Executor, error)
```

### Implementation

```go
package security

type executor struct {
    config    *SecurityDescriptor
    context   *ExecutionContext
    logger    *logger.Logger
    cache     *Cache

    // Component instances
    signer      signing.Signer
    sbomGen     sbom.Generator
    provenanceGen provenance.Generator
    scanners    []scan.Scanner
    tools       *tools.Installer
}

// Execute implements Executor
func (e *executor) Execute(ctx context.Context, image ImageReference) (*SecurityResult, error) {
    result := &SecurityResult{
        Image:     image,
        StartedAt: time.Now(),
    }

    // 1. Validate configuration
    if err := e.ValidateConfig(e.config); err != nil {
        return nil, err
    }

    // 2. Check tool availability
    if err := e.tools.CheckAllTools(ctx, e.config); err != nil {
        return nil, err
    }

    // 3. Execute scanning (fail-fast)
    if e.config.Scan != nil && e.config.Scan.Enabled {
        scanResults, err := e.executeScan(ctx, image)
        result.ScanResults = scanResults
        result.Scanned = true

        if err != nil {
            // Fail-fast on critical vulnerabilities
            return result, err
        }
    }

    // 4. Execute signing
    if e.config.Signing != nil && e.config.Signing.Enabled {
        signResult, err := e.executeSign(ctx, image)
        result.SignResult = signResult
        result.Signed = err == nil

        if err != nil {
            result.Errors = append(result.Errors, err)
            // Continue (fail-open)
        }
    }

    // 5. Execute SBOM generation (can parallelize)
    if e.config.SBOM != nil && e.config.SBOM.Enabled {
        sbomResult, err := e.executeSBOM(ctx, image)
        result.SBOM = sbomResult
        result.SBOMGenerated = err == nil

        if err != nil {
            result.Errors = append(result.Errors, err)
            // Continue (fail-open)
        }
    }

    // 6. Execute provenance generation
    if e.config.Provenance != nil && e.config.Provenance.Enabled {
        provenanceResult, err := e.executeProvenance(ctx, image)
        result.Provenance = provenanceResult
        result.ProvenanceGenerated = err == nil

        if err != nil {
            result.Errors = append(result.Errors, err)
            // Continue (fail-open)
        }
    }

    result.FinishedAt = time.Now()
    result.Duration = result.FinishedAt.Sub(result.StartedAt)

    return result, nil
}

// ExecuteWithPulumi implements Executor for Pulumi integration
func (e *executor) ExecuteWithPulumi(
    pulumiCtx *sdk.Context,
    dockerImage *docker.Image,
    config *SecurityDescriptor,
) ([]sdk.ResourceOption, error) {
    // Convert docker.Image to ImageReference
    imageRef := e.extractImageReference(dockerImage)

    var resources []sdk.Resource
    var err error

    // Execute scanning with Pulumi Command
    if config.Scan != nil && config.Scan.Enabled {
        scanCmd, err := e.createScanCommand(pulumiCtx, dockerImage, imageRef, config.Scan)
        if err != nil {
            return nil, err
        }
        resources = append(resources, scanCmd)
    }

    // Execute signing with Pulumi Command
    if config.Signing != nil && config.Signing.Enabled {
        signCmd, err := e.createSignCommand(pulumiCtx, dockerImage, imageRef, config.Signing)
        if err != nil {
            return nil, err
        }
        resources = append(resources, signCmd)
    }

    // Execute SBOM generation
    if config.SBOM != nil && config.SBOM.Enabled {
        sbomCmd, err := e.createSBOMCommand(pulumiCtx, dockerImage, imageRef, config.SBOM)
        if err != nil {
            return nil, err
        }
        resources = append(resources, sbomCmd)
    }

    // Execute provenance generation
    if config.Provenance != nil && config.Provenance.Enabled {
        provenanceCmd, err := e.createProvenanceCommand(pulumiCtx, dockerImage, imageRef, config.Provenance)
        if err != nil {
            return nil, err
        }
        resources = append(resources, provenanceCmd)
    }

    // Return dependency options
    return []sdk.ResourceOption{sdk.DependsOn(resources)}, nil
}

// ValidateConfig implements Executor
func (e *executor) ValidateConfig(config *SecurityDescriptor) error {
    if config == nil {
        return errors.New("security config is nil")
    }

    // Validate signing config
    if config.Signing != nil && config.Signing.Enabled {
        if !config.Signing.Keyless {
            if config.Signing.PrivateKey == "" {
                return errors.New("signing.privateKey required when keyless=false")
            }
        }
    }

    // Validate SBOM config
    if config.SBOM != nil && config.SBOM.Enabled {
        validFormats := []string{"cyclonedx-json", "cyclonedx-xml", "spdx-json", "spdx-tag-value", "syft-json"}
        if config.SBOM.Format != "" {
            if !contains(validFormats, config.SBOM.Format) {
                return fmt.Errorf("invalid sbom.format: %s (valid: %v)", config.SBOM.Format, validFormats)
            }
        }
    }

    // Validate scan config
    if config.Scan != nil && config.Scan.Enabled {
        validTools := []string{"grype", "trivy"}
        for _, tool := range config.Scan.Tools {
            if !contains(validTools, tool.Name) {
                return fmt.Errorf("invalid scan tool: %s (valid: %v)", tool.Name, validTools)
            }
        }
    }

    return nil
}
```

---

## Signing API

### Interface

```go
package signing

// Signer signs and verifies container images
type Signer interface {
    // Sign signs the container image
    Sign(ctx context.Context, ref ImageReference, opts SignOptions) (*SignResult, error)

    // Verify verifies image signature
    Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*VerifyResult, error)

    // GetPublicKey returns the public key (if applicable)
    GetPublicKey() (string, error)
}

// NewSigner creates a signer based on configuration
func NewSigner(config *SigningConfig, context *ExecutionContext, logger *logger.Logger) (Signer, error)

// NewKeylessSigner creates OIDC keyless signer
func NewKeylessSigner(logger *logger.Logger) Signer

// NewKeyBasedSigner creates key-based signer
func NewKeyBasedSigner(logger *logger.Logger) Signer
```

### Types

```go
// SignOptions contains options for signing
type SignOptions struct {
    // Keyless options
    OIDCToken  string
    OIDCIssuer string

    // Key-based options
    PrivateKey string
    Password   string

    // Common options
    Registry    RegistryAuth
    Annotations map[string]string

    // Rekor options
    RekorURL string // Default: https://rekor.sigstore.dev
}

// SignResult contains signing result
type SignResult struct {
    Digest      string            // Signed image digest
    Signature   string            // Signature string
    Bundle      string            // Signature bundle (for verification)
    RekorEntry  string            // Rekor transparency log entry URL
    Metadata    map[string]string // Additional metadata
    SignedAt    time.Time
}

// VerifyOptions contains options for verification
type VerifyOptions struct {
    // Keyless verification
    OIDCIssuer     string
    IdentityRegexp string // Regexp to match certificate identity

    // Key-based verification
    PublicKey string

    // Common options
    Registry RegistryAuth
}

// VerifyResult contains verification result
type VerifyResult struct {
    Valid       bool
    Claims      map[string]interface{} // Claims from certificate/signature
    RekorEntry  string                 // Rekor entry verified
    Error       string                 // Error message if invalid
    VerifiedAt  time.Time
}
```

---

## SBOM API

### Interface

```go
package sbom

// Generator generates Software Bill of Materials
type Generator interface {
    // Generate creates SBOM for image
    Generate(ctx context.Context, ref ImageReference, opts GenerateOptions) (*SBOM, error)

    // SupportedFormats returns list of supported output formats
    SupportedFormats() []string
}

// Attacher attaches SBOM as attestation to images
type Attacher interface {
    // Attach attaches SBOM as signed attestation
    Attach(ctx context.Context, ref ImageReference, sbom *SBOM, opts AttachOptions) error

    // Verify verifies SBOM attestation
    Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*SBOM, error)
}

// NewGenerator creates SBOM generator
func NewGenerator(generatorType string, logger *logger.Logger) (Generator, error)

// NewAttacher creates SBOM attacher
func NewAttacher(signer signing.Signer, logger *logger.Logger) Attacher
```

### Types

```go
// GenerateOptions contains options for SBOM generation
type GenerateOptions struct {
    Format      string   // cyclonedx-json, spdx-json, syft-json
    OutputPath  string   // Local file path (optional)
    Catalogers  []string // Specific catalogers to use
    Scope       string   // all-layers, squashed
}

// SBOM represents a Software Bill of Materials
type SBOM struct {
    Format      string       // Format used
    Content     []byte       // Raw SBOM content
    Digest      string       // SBOM content hash (SHA256)
    ImageDigest string       // Image digest
    GeneratedAt time.Time
    Metadata    SBOMMetadata
}

// SBOMMetadata contains SBOM generation metadata
type SBOMMetadata struct {
    ToolName     string
    ToolVersion  string
    PackageCount int
    Format       string
}

// AttachOptions contains options for attaching SBOM
type AttachOptions struct {
    Sign     bool         // Sign the attestation
    Keyless  bool         // Use keyless signing
    KeyPath  string       // Private key path (key-based)
    Env      map[string]string // Environment variables
}

// VerifyOptions contains options for verifying SBOM attestation
type VerifyOptions struct {
    OIDCIssuer     string // For keyless verification
    IdentityRegexp string // Identity regex
    PublicKey      string // For key-based verification
}
```

---

## Provenance API

### Interface

```go
package provenance

// Generator generates SLSA provenance attestation
type Generator interface {
    // Generate creates provenance for image
    Generate(ctx context.Context, ref ImageReference, opts ProvenanceOptions) (*Provenance, error)

    // Attach attaches provenance as signed attestation
    Attach(ctx context.Context, ref ImageReference, provenance *Provenance, opts AttachOptions) error

    // Verify verifies provenance attestation
    Verify(ctx context.Context, ref ImageReference, opts VerifyOptions) (*Provenance, error)
}

// NewGenerator creates provenance generator
func NewGenerator(context *ExecutionContext, logger *logger.Logger) Generator
```

### Types

```go
// ProvenanceOptions contains options for provenance generation
type ProvenanceOptions struct {
    // Build information
    Dockerfile  string
    BuildArgs   map[string]string

    // Materials
    IncludeMaterials bool

    // Environment
    IncludeEnv bool

    // Custom metadata
    CustomMetadata map[string]interface{}
}

// Provenance represents SLSA provenance attestation
type Provenance struct {
    Format      string    // slsa-v1.0
    Content     []byte    // Raw provenance JSON
    ImageDigest string    // Image digest
    GeneratedAt time.Time
    Metadata    ProvenanceMetadata
}

// ProvenanceMetadata contains provenance generation metadata
type ProvenanceMetadata struct {
    SLSAVersion string
    BuilderID   string
    BuildType   string
}

// AttachOptions contains options for attaching provenance
type AttachOptions struct {
    Sign     bool
    Keyless  bool
    KeyPath  string
    Env      map[string]string
}

// VerifyOptions contains options for verifying provenance
type VerifyOptions struct {
    OIDCIssuer     string
    IdentityRegexp string
    PublicKey      string

    // SLSA level verification
    MinSLSALevel int // Minimum SLSA level required
}

// SLSA Provenance v1.0 Types

// SLSAProvenance represents SLSA provenance structure
type SLSAProvenance struct {
    Type      string         `json:"_type"`
    Predicate SLSAPredicate  `json:"predicate"`
    Subject   []Subject      `json:"subject"`
}

// Subject represents the artifact being attested
type Subject struct {
    Name   string            `json:"name"`
    Digest map[string]string `json:"digest"`
}

// SLSAPredicate contains the provenance predicate
type SLSAPredicate struct {
    BuildDefinition BuildDefinition `json:"buildDefinition"`
    RunDetails      RunDetails      `json:"runDetails"`
}

// BuildDefinition describes how the artifact was built
type BuildDefinition struct {
    BuildType            string                 `json:"buildType"`
    ExternalParameters   map[string]interface{} `json:"externalParameters"`
    InternalParameters   map[string]interface{} `json:"internalParameters"`
    ResolvedDependencies []Material             `json:"resolvedDependencies"`
}

// Material represents a build material (source, dependency)
type Material struct {
    URI    string            `json:"uri"`
    Digest map[string]string `json:"digest"`
}

// RunDetails contains information about the build execution
type RunDetails struct {
    Builder  Builder  `json:"builder"`
    Metadata Metadata `json:"metadata"`
}

// Builder identifies the build platform
type Builder struct {
    ID      string            `json:"id"`
    Version map[string]string `json:"version,omitempty"`
}

// Metadata contains build execution metadata
type Metadata struct {
    InvocationID string `json:"invocationID"`
    StartedOn    string `json:"startedOn"`
    FinishedOn   string `json:"finishedOn,omitempty"`
}
```

---

## Scanning API

### Interface

```go
package scan

// Scanner performs vulnerability scanning on images
type Scanner interface {
    // Scan performs vulnerability scan on image
    Scan(ctx context.Context, ref ImageReference, opts ScanOptions) (*ScanResult, error)

    // Name returns scanner name
    Name() string

    // Version returns scanner version
    Version() (string, error)
}

// PolicyEnforcer enforces vulnerability policies
type PolicyEnforcer interface {
    // Enforce applies policy to scan results
    Enforce(ctx context.Context, results []*ScanResult, config *ScanConfig) error
}

// NewScanner creates scanner for given tool
func NewScanner(tool string, logger *logger.Logger) (Scanner, error)

// NewPolicyEnforcer creates policy enforcer
func NewPolicyEnforcer(logger *logger.Logger) PolicyEnforcer
```

### Types

```go
// ScanOptions contains options for scanning
type ScanOptions struct {
    FailOn     Severity // Fail on this severity or higher
    WarnOn     Severity // Warn on this severity or higher
    Scope      string   // all-layers, squashed
    OutputPath string   // Save results to file

    // Database options
    DBPath     string   // Custom vulnerability database path
    DBUpdate   bool     // Update database before scanning
}

// ScanResult contains vulnerability scan results
type ScanResult struct {
    Scanner         string          // grype, trivy
    Version         string          // Scanner version
    ImageDigest     string          // Scanned image digest
    Vulnerabilities []Vulnerability // Found vulnerabilities
    Summary         VulnerabilitySummary
    ScannedAt       time.Time
    Duration        time.Duration
}

// Vulnerability represents a single vulnerability
type Vulnerability struct {
    ID          string   // CVE-2023-1234
    Severity    Severity // critical, high, medium, low
    Package     string   // Package name
    Version     string   // Installed version
    FixedIn     string   // Fixed version (if available)
    Description string   // Vulnerability description
    URLs        []string // Reference URLs
    CVSS        CVSS     // CVSS scores
}

// CVSS represents CVSS scoring information
type CVSS struct {
    Version string  // 2.0, 3.0, 3.1
    Score   float64 // 0.0 - 10.0
    Vector  string  // CVSS vector string
}

// VulnerabilitySummary summarizes vulnerabilities by severity
type VulnerabilitySummary struct {
    Critical int
    High     int
    Medium   int
    Low      int
    Unknown  int
    Total    int
}

// Severity represents vulnerability severity level
type Severity string

const (
    SeverityCritical Severity = "critical"
    SeverityHigh     Severity = "high"
    SeverityMedium   Severity = "medium"
    SeverityLow      Severity = "low"
    SeverityUnknown  Severity = "unknown"
)

// Compare compares two severities (returns -1, 0, 1)
func (s Severity) Compare(other Severity) int

// IsHigherThan returns true if s is higher severity than other
func (s Severity) IsHigherThan(other Severity) bool
```

---

## Tool Management API

### Interface

```go
package tools

// Installer checks and validates tool installations
type Installer interface {
    // CheckInstalled verifies tool is installed and meets version requirements
    CheckInstalled(ctx context.Context, tool ToolMetadata) (bool, string, error)

    // CheckAllTools validates all required tools for configuration
    CheckAllTools(ctx context.Context, config *SecurityDescriptor) error

    // GetInstallInstructions returns installation instructions for tool
    GetInstallInstructions(tool string) string
}

// CommandExecutor executes external commands
type CommandExecutor interface {
    // Execute runs command with given environment
    Execute(ctx context.Context, cmd []string, env map[string]string) ([]byte, error)

    // ExecuteWithTimeout runs command with timeout
    ExecuteWithTimeout(ctx context.Context, cmd []string, env map[string]string, timeout time.Duration) ([]byte, error)
}

// NewInstaller creates tool installer
func NewInstaller(logger *logger.Logger) Installer

// NewCommandExecutor creates command executor
func NewCommandExecutor(logger *logger.Logger) CommandExecutor
```

### Types

```go
// ToolMetadata contains tool information
type ToolMetadata struct {
    Name        string // Display name
    Command     string // Command name
    MinVersion  string // Minimum required version
    InstallURL  string // Installation instructions URL
    Required    bool   // Whether tool is required
}

// ToolRegistry contains metadata for all supported tools
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

## CLI Commands API

### Image Commands

```go
package cmd_image

// SignCommand signs a container image
type SignCommand struct {
    Image    string // Image reference
    Keyless  bool   // Use keyless signing
    Key      string // Private key path (key-based)
    Password string // Key password
}

func NewSignCommand() *cobra.Command

// VerifyCommand verifies image signature
type VerifyCommand struct {
    Image          string // Image reference
    OIDCIssuer     string // OIDC issuer for keyless
    IdentityRegexp string // Identity regexp
    PublicKey      string // Public key (key-based)
}

func NewVerifyCommand() *cobra.Command

// ScanCommand scans image for vulnerabilities
type ScanCommand struct {
    Image      string // Image reference
    Tool       string // Scanner to use (grype, trivy, all)
    FailOn     string // Fail on severity
    OutputPath string // Save results to file
}

func NewScanCommand() *cobra.Command
```

### SBOM Commands

```go
package cmd_sbom

// GenerateCommand generates SBOM for image
type GenerateCommand struct {
    Image      string // Image reference
    Format     string // cyclonedx-json, spdx-json
    OutputPath string // Save SBOM to file
}

func NewGenerateCommand() *cobra.Command

// AttachCommand attaches SBOM as attestation
type AttachCommand struct {
    Image   string // Image reference
    SBOM    string // SBOM file path
    Sign    bool   // Sign attestation
    Keyless bool   // Use keyless signing
}

func NewAttachCommand() *cobra.Command

// VerifyCommand verifies SBOM attestation
type VerifyCommand struct {
    Image          string // Image reference
    OIDCIssuer     string // OIDC issuer
    IdentityRegexp string // Identity regexp
    OutputPath     string // Save verified SBOM
}

func NewVerifyCommand() *cobra.Command
```

### Provenance Commands

```go
package cmd_provenance

// AttachCommand attaches provenance attestation
type AttachCommand struct {
    Image        string // Image reference
    SourceRepo   string // Source repository
    GitSHA       string // Git commit SHA
    WorkflowName string // CI workflow name
    Sign         bool   // Sign attestation
    Keyless      bool   // Use keyless signing
}

func NewAttachCommand() *cobra.Command

// VerifyCommand verifies provenance attestation
type VerifyCommand struct {
    Image          string // Image reference
    OIDCIssuer     string // OIDC issuer
    IdentityRegexp string // Identity regexp
    MinSLSALevel   int    // Minimum SLSA level
}

func NewVerifyCommand() *cobra.Command
```

### Release Commands

```go
package cmd_release

// CreateCommand executes integrated release workflow
type CreateCommand struct {
    Stack       string // Stack name
    Environment string // Environment (production, staging)
    Version     string // Release version

    // Security options (optional overrides)
    Sign        bool   // Enable signing
    SBOM        bool   // Enable SBOM
    Scan        bool   // Enable scanning
}

func NewCreateCommand() *cobra.Command

// CreateOptions contains options for release creation
type CreateOptions struct {
    Stack       string
    Environment string
    Version     string

    // Security overrides
    SecurityConfig *SecurityDescriptor
}

// ExecuteRelease executes full release workflow
func ExecuteRelease(ctx context.Context, opts CreateOptions) (*ReleaseResult, error)

// ReleaseResult contains release execution result
type ReleaseResult struct {
    Images          []ImageReference
    SecurityResults []*SecurityResult
    Success         bool
    Duration        time.Duration
    Errors          []error
}
```

---

## Error Types

```go
package security

// Common error types

var (
    // Tool errors
    ErrToolNotFound     = errors.New("required tool not found")
    ErrToolVersion      = errors.New("tool version incompatible")
    ErrToolExecution    = errors.New("tool execution failed")

    // Configuration errors
    ErrInvalidConfig    = errors.New("invalid configuration")
    ErrMissingKey       = errors.New("signing key not provided")
    ErrMissingOIDC      = errors.New("OIDC token not available")

    // Operation errors
    ErrSigningFailed    = errors.New("image signing failed")
    ErrVerifyFailed     = errors.New("signature verification failed")
    ErrSBOMGeneration   = errors.New("SBOM generation failed")
    ErrProvenanceGen    = errors.New("provenance generation failed")
    ErrScanFailed       = errors.New("vulnerability scan failed")

    // Policy errors
    ErrCriticalVulns    = errors.New("critical vulnerabilities found")
    ErrPolicyViolation  = errors.New("security policy violation")
)

// SecurityError wraps errors with additional context
type SecurityError struct {
    Op       string // Operation that failed
    Image    ImageReference
    Err      error
    Metadata map[string]string
}

func (e *SecurityError) Error() string {
    return fmt.Sprintf("%s failed for %s: %v", e.Op, e.Image.String(), e.Err)
}

func (e *SecurityError) Unwrap() error {
    return e.Err
}

// NewSecurityError creates a new security error
func NewSecurityError(op string, image ImageReference, err error) *SecurityError {
    return &SecurityError{
        Op:       op,
        Image:    image,
        Err:      err,
        Metadata: make(map[string]string),
    }
}
```

---

## Integration with Existing API

### Modified Types in `pkg/api/client.go`

```go
// StackConfigSingleImage (existing type, add SecurityDescriptor field)
type StackConfigSingleImage struct {
    // ... existing fields ...

    // Security configuration (new)
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}

// ComposeService (existing type, add SecurityDescriptor field)
type ComposeService struct {
    // ... existing fields ...

    // Security configuration (new)
    Security *SecurityDescriptor `json:"security,omitempty" yaml:"security,omitempty"`
}
```

### New File `pkg/api/security_config.go`

```go
package api

// SecurityDescriptor defines security operations for container images
type SecurityDescriptor struct {
    Signing    *SigningConfig    `json:"signing,omitempty" yaml:"signing,omitempty"`
    SBOM       *SBOMConfig       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
    Provenance *ProvenanceConfig `json:"provenance,omitempty" yaml:"provenance,omitempty"`
    Scan       *ScanConfig       `json:"scan,omitempty" yaml:"scan,omitempty"`
}

// ... (other config types as defined in component-design.md)
```

---

## Summary

This API contract document defines:

1. **Core Types** - ImageReference, RegistryAuth, SecurityResult
2. **Executor API** - Main orchestrator interface
3. **Signing API** - Image signing and verification
4. **SBOM API** - SBOM generation and attestation
5. **Provenance API** - SLSA provenance generation
6. **Scanning API** - Vulnerability scanning and policy enforcement
7. **Tool Management API** - External tool validation
8. **CLI Commands API** - Command-line interface contracts

All interfaces are designed for:
- **Testability** - Easy to mock for unit tests
- **Extensibility** - New implementations can be added
- **Type Safety** - Strong typing with Go interfaces
- **Error Handling** - Consistent error types and wrapping

**Next Steps:**
- Review [Integration & Data Flow](./integration-dataflow.md) for execution flow
- Review [Implementation Plan](./implementation-plan.md) for development tasks

---

**Status:** ✅ API Contracts Complete
**Related Documents:** [Architecture Overview](./README.md) | [Component Design](./component-design.md) | [Integration & Data Flow](./integration-dataflow.md)
