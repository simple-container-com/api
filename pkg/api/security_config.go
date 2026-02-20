package api

// SecurityDescriptor defines security configuration for container images
// This is the API-level representation that maps to pkg/security types
type SecurityDescriptor struct {
	Enabled    bool                  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Signing    *SigningDescriptor    `json:"signing,omitempty" yaml:"signing,omitempty"`
	SBOM       *SBOMDescriptor       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
	Provenance *ProvenanceDescriptor `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Scan       *ScanDescriptor       `json:"scan,omitempty" yaml:"scan,omitempty"`
}

// SigningDescriptor configures image signing
type SigningDescriptor struct {
	Enabled    bool              `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Provider   string            `json:"provider,omitempty" yaml:"provider,omitempty"` // Default: "sigstore"
	Keyless    bool              `json:"keyless,omitempty" yaml:"keyless,omitempty"`   // Default: true
	PrivateKey string            `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
	PublicKey  string            `json:"publicKey,omitempty" yaml:"publicKey,omitempty"`
	Password   string            `json:"password,omitempty" yaml:"password,omitempty"`
	Required   bool              `json:"required,omitempty" yaml:"required,omitempty"`
	Verify     *VerifyDescriptor `json:"verify,omitempty" yaml:"verify,omitempty"`
}

// VerifyDescriptor configures signature verification
type VerifyDescriptor struct {
	Enabled        bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	OIDCIssuer     string `json:"oidcIssuer,omitempty" yaml:"oidcIssuer,omitempty"`
	IdentityRegexp string `json:"identityRegexp,omitempty" yaml:"identityRegexp,omitempty"`
}

// SBOMDescriptor configures SBOM generation
type SBOMDescriptor struct {
	Enabled   bool              `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Format    string            `json:"format,omitempty" yaml:"format,omitempty"`       // Default: "cyclonedx-json"
	Generator string            `json:"generator,omitempty" yaml:"generator,omitempty"` // Default: "syft"
	Output    *OutputDescriptor `json:"output,omitempty" yaml:"output,omitempty"`
	Attach    *AttachDescriptor `json:"attach,omitempty" yaml:"attach,omitempty"`
	Required  bool              `json:"required,omitempty" yaml:"required,omitempty"`
}

// OutputDescriptor configures output destinations
type OutputDescriptor struct {
	Local    string `json:"local,omitempty" yaml:"local,omitempty"`       // Local file path
	Registry bool   `json:"registry,omitempty" yaml:"registry,omitempty"` // Upload to registry
}

// AttachDescriptor configures attestation attachment
type AttachDescriptor struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"` // Default: true
	Sign    bool `json:"sign,omitempty" yaml:"sign,omitempty"`       // Sign the attestation
}

// ProvenanceDescriptor configures SLSA provenance
type ProvenanceDescriptor struct {
	Enabled       bool                `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Format        string              `json:"format,omitempty" yaml:"format,omitempty"` // Default: "slsa-v1.0"
	Output        *OutputDescriptor   `json:"output,omitempty" yaml:"output,omitempty"`
	IncludeGit    bool                `json:"includeGit,omitempty" yaml:"includeGit,omitempty"`
	IncludeDocker bool                `json:"includeDockerfile,omitempty" yaml:"includeDockerfile,omitempty"`
	Required      bool                `json:"required,omitempty" yaml:"required,omitempty"`
	Builder       *BuilderDescriptor  `json:"builder,omitempty" yaml:"builder,omitempty"`
	Metadata      *MetadataDescriptor `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// BuilderDescriptor configures builder identification
type BuilderDescriptor struct {
	ID string `json:"id,omitempty" yaml:"id,omitempty"` // Auto-detected from CI if not specified
}

// MetadataDescriptor configures metadata collection
type MetadataDescriptor struct {
	IncludeEnv       bool `json:"includeEnv,omitempty" yaml:"includeEnv,omitempty"`
	IncludeMaterials bool `json:"includeMaterials,omitempty" yaml:"includeMaterials,omitempty"`
}

// ScanDescriptor configures vulnerability scanning
type ScanDescriptor struct {
	Enabled  bool                 `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Tools    []ScanToolDescriptor `json:"tools,omitempty" yaml:"tools,omitempty"`
	FailOn   string               `json:"failOn,omitempty" yaml:"failOn,omitempty"` // "critical", "high", "medium", "low"
	WarnOn   string               `json:"warnOn,omitempty" yaml:"warnOn,omitempty"` // "critical", "high", "medium", "low"
	Required bool                 `json:"required,omitempty" yaml:"required,omitempty"`
}

// ScanToolDescriptor configures a specific scanning tool
type ScanToolDescriptor struct {
	Name     string `json:"name,omitempty" yaml:"name,omitempty"` // "grype", "trivy"
	Enabled  bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
	FailOn   string `json:"failOn,omitempty" yaml:"failOn,omitempty"`
	WarnOn   string `json:"warnOn,omitempty" yaml:"warnOn,omitempty"`
}

// DefaultSecurityDescriptor returns a default security descriptor
func DefaultSecurityDescriptor() *SecurityDescriptor {
	return &SecurityDescriptor{
		Enabled: false,
		Signing: &SigningDescriptor{
			Enabled:  false,
			Keyless:  true,
			Required: false,
		},
		SBOM: &SBOMDescriptor{
			Enabled:   false,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Output: &OutputDescriptor{
				Registry: true,
			},
			Attach: &AttachDescriptor{
				Enabled: true,
				Sign:    true,
			},
			Required: false,
		},
		Provenance: &ProvenanceDescriptor{
			Enabled:    false,
			Format:     "slsa-v1.0",
			IncludeGit: true,
			Required:   false,
			Metadata: &MetadataDescriptor{
				IncludeEnv:       false,
				IncludeMaterials: true,
			},
		},
		Scan: &ScanDescriptor{
			Enabled:  false,
			FailOn:   "critical",
			Required: false,
			Tools: []ScanToolDescriptor{
				{
					Name:     "grype",
					Enabled:  true,
					Required: true,
					FailOn:   "critical",
				},
			},
		},
	}
}
