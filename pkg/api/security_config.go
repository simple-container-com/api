package api

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

// VerifyConfig configures signature verification
type VerifyConfig struct {
	Enabled        bool   `json:"enabled" yaml:"enabled"`
	OIDCIssuer     string `json:"oidcIssuer,omitempty" yaml:"oidcIssuer,omitempty"`
	IdentityRegexp string `json:"identityRegexp,omitempty" yaml:"identityRegexp,omitempty"`
}

// SBOMConfig configures SBOM generation
type SBOMConfig struct {
	Enabled   bool          `json:"enabled" yaml:"enabled"`
	Format    string        `json:"format,omitempty" yaml:"format,omitempty"`       // Default: "cyclonedx-json"
	Generator string        `json:"generator,omitempty" yaml:"generator,omitempty"` // Default: "syft"
	Attach    *AttachConfig `json:"attach,omitempty" yaml:"attach,omitempty"`
	Output    *OutputConfig `json:"output,omitempty" yaml:"output,omitempty"`
}

// AttachConfig configures attestation attachment
type AttachConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"` // Default: true
	Sign    bool `json:"sign" yaml:"sign"`       // Default: true
}

// OutputConfig configures output destinations
type OutputConfig struct {
	Local    string `json:"local,omitempty" yaml:"local,omitempty"` // Local directory
	Registry bool   `json:"registry" yaml:"registry"`               // Default: true
}

// ProvenanceConfig configures SLSA provenance
type ProvenanceConfig struct {
	Enabled  bool            `json:"enabled" yaml:"enabled"`
	Version  string          `json:"version,omitempty" yaml:"version,omitempty"` // Default: "1.0"
	Builder  *BuilderConfig  `json:"builder,omitempty" yaml:"builder,omitempty"`
	Metadata *MetadataConfig `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// BuilderConfig configures builder information
type BuilderConfig struct {
	ID string `json:"id,omitempty" yaml:"id,omitempty"` // Auto-detected from CI
}

// MetadataConfig configures provenance metadata
type MetadataConfig struct {
	IncludeEnv       bool `json:"includeEnv" yaml:"includeEnv"`             // Default: false
	IncludeMaterials bool `json:"includeMaterials" yaml:"includeMaterials"` // Default: true
}

// ScanConfig configures vulnerability scanning
type ScanConfig struct {
	Enabled bool             `json:"enabled" yaml:"enabled"`
	Tools   []ScanToolConfig `json:"tools,omitempty" yaml:"tools,omitempty"`
}

// ScanToolConfig configures individual scanning tools
type ScanToolConfig struct {
	Name     string   `json:"name" yaml:"name"`                         // grype, trivy
	Required bool     `json:"required" yaml:"required"`                 // Default: true for grype
	FailOn   Severity `json:"failOn,omitempty" yaml:"failOn,omitempty"` // critical, high, medium, low
	WarnOn   Severity `json:"warnOn,omitempty" yaml:"warnOn,omitempty"`
}

// Severity represents vulnerability severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)
