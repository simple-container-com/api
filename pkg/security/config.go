package security

import (
	"strconv"
	"time"

	"github.com/simple-container-com/api/pkg/api"
)

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
		if r.Registry == "" {
			return r.Repository + "@" + r.Digest
		}
		return r.Registry + "/" + r.Repository + "@" + r.Digest
	}
	if r.Registry == "" {
		return r.Repository + ":" + r.Tag
	}
	return r.Registry + "/" + r.Repository + ":" + r.Tag
}

// WithDigest returns a new reference with digest
func (r ImageReference) WithDigest(digest string) ImageReference {
	return ImageReference{
		Registry:   r.Registry,
		Repository: r.Repository,
		Digest:     digest,
	}
}

// RegistryAuth holds registry authentication credentials
type RegistryAuth struct {
	Username string
	Password string
	Token    string // For token-based auth
}

// SignOptions contains options for signing operations
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
}

// SignResult contains the result of a signing operation
type SignResult struct {
	Digest     string            // Signed image digest
	Signature  string            // Signature string
	Bundle     string            // Signature bundle (for verification)
	RekorEntry string            // Rekor transparency log entry (keyless)
	Metadata   map[string]string // Additional metadata
}

// VerifyOptions contains options for verification operations
type VerifyOptions struct {
	OIDCIssuer     string
	IdentityRegexp string
	PublicKey      string
}

// VerifyResult contains the result of a verification operation
type VerifyResult struct {
	Valid  bool
	Claims map[string]interface{}
	Error  string
}

// GenerateOptions contains options for SBOM generation
type GenerateOptions struct {
	Format     string   // cyclonedx-json, spdx-json, syft-json
	OutputPath string   // Local file path (optional)
	Catalogers []string // Specific catalogers to use
	Scope      string   // all-layers, squashed
}

// SBOM represents a Software Bill of Materials
type SBOM struct {
	Format      string // Format used
	Content     []byte // Raw SBOM content
	Digest      string // SBOM content hash
	ImageDigest string // Image digest
	GeneratedAt time.Time
	Metadata    SBOMMetadata
}

// SBOMMetadata contains metadata about SBOM generation
type SBOMMetadata struct {
	ToolName     string
	ToolVersion  string
	PackageCount int
}

// AttachOptions contains options for attestation attachment
type AttachOptions struct {
	Sign    bool
	Keyless bool
	KeyPath string
	Env     map[string]string
}

// ProvenanceOptions contains options for provenance generation
type ProvenanceOptions struct {
	Dockerfile string
	BuildArgs  map[string]string
}

// Provenance represents SLSA provenance attestation
type Provenance struct {
	Format      string // slsa-v1.0
	Content     []byte // Raw provenance content
	ImageDigest string // Image digest
	GeneratedAt time.Time
}

// ScanOptions contains options for vulnerability scanning
type ScanOptions struct {
	FailOn     api.Severity // critical, high, medium, low
	WarnOn     api.Severity // Severity to warn (not fail)
	Scope      string       // all-layers, squashed
	OutputPath string       // Save results to file
}

// ScanResult contains the result of a vulnerability scan
type ScanResult struct {
	Scanner         string          // grype, trivy
	Version         string          // Scanner version
	ImageDigest     string          // Scanned image digest
	Vulnerabilities []Vulnerability // Found vulnerabilities
	Summary         VulnerabilitySummary
	ScannedAt       time.Time
}

// Vulnerability represents a single vulnerability
type Vulnerability struct {
	ID          string       // CVE-2023-1234
	Severity    api.Severity // critical, high, medium, low
	Package     string       // Package name
	Version     string       // Installed version
	FixedIn     string       // Fixed version (if available)
	Description string       // Vulnerability description
	URLs        []string     // Reference URLs
}

// VulnerabilitySummary contains vulnerability counts by severity
type VulnerabilitySummary struct {
	Critical int
	High     int
	Medium   int
	Low      int
	Total    int
}

// SecurityResult contains results from all security operations
type SecurityResult struct {
	Image ImageReference

	// Operation results
	Signed     bool
	SignResult *SignResult

	SBOMGenerated bool
	SBOM          *SBOM

	ProvenanceGenerated bool
	Provenance          *Provenance

	Scanned     bool
	ScanResults []*ScanResult

	// Timing
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration

	// Errors and warnings
	Errors   []error
	Warnings []string
}

// HasCriticalIssues returns true if any operation found critical issues
func (r *SecurityResult) HasCriticalIssues() bool {
	for _, scanResult := range r.ScanResults {
		if scanResult.Summary.Critical > 0 {
			return true
		}
	}
	return false
}

// Summary returns human-readable summary
func (r *SecurityResult) Summary() string {
	summary := "Security operations completed:\n"
	if r.Signed {
		summary += "  ✓ Image signed\n"
	}
	if r.SBOMGenerated {
		summary += "  ✓ SBOM generated\n"
	}
	if r.ProvenanceGenerated {
		summary += "  ✓ Provenance generated\n"
	}
	if r.Scanned {
		summary += "  ✓ Vulnerability scan completed\n"
		for _, scanResult := range r.ScanResults {
			summary += "    " + scanResult.Scanner + ": " +
				strconv.Itoa(scanResult.Summary.Critical) + " critical, " +
				strconv.Itoa(scanResult.Summary.High) + " high\n"
		}
	}
	return summary
}
