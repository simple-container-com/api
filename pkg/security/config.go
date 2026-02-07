package security

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// SecurityConfig contains comprehensive configuration for all security operations
type SecurityConfig struct {
	Enabled    bool              `json:"enabled" yaml:"enabled"`
	Signing    *signing.Config   `json:"signing,omitempty" yaml:"signing,omitempty"`
	SBOM       *SBOMConfig       `json:"sbom,omitempty" yaml:"sbom,omitempty"`
	Provenance *ProvenanceConfig `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Scan       *ScanConfig       `json:"scan,omitempty" yaml:"scan,omitempty"`
}

// SBOMConfig configures SBOM generation
type SBOMConfig struct {
	Enabled   bool          `json:"enabled" yaml:"enabled"`
	Format    string        `json:"format,omitempty" yaml:"format,omitempty"`       // Default: "cyclonedx-json"
	Generator string        `json:"generator,omitempty" yaml:"generator,omitempty"` // Default: "syft"
	Output    *OutputConfig `json:"output,omitempty" yaml:"output,omitempty"`
	Attach    *AttachConfig `json:"attach,omitempty" yaml:"attach,omitempty"`
	Required  bool          `json:"required,omitempty" yaml:"required,omitempty"` // Fail if SBOM generation fails
}

// OutputConfig configures output destinations
type OutputConfig struct {
	Local    string `json:"local,omitempty" yaml:"local,omitempty"`       // Local file path
	Registry bool   `json:"registry,omitempty" yaml:"registry,omitempty"` // Upload to registry
}

// AttachConfig configures attestation attachment
type AttachConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"` // Default: true
	Sign    bool `json:"sign" yaml:"sign"`       // Sign the attestation
}

// ProvenanceConfig configures SLSA provenance generation
type ProvenanceConfig struct {
	Enabled       bool            `json:"enabled" yaml:"enabled"`
	Format        string          `json:"format,omitempty" yaml:"format,omitempty"` // Default: "slsa-v1.0"
	Output        *OutputConfig   `json:"output,omitempty" yaml:"output,omitempty"`
	IncludeGit    bool            `json:"includeGit,omitempty" yaml:"includeGit,omitempty"`               // Include git metadata
	IncludeDocker bool            `json:"includeDockerfile,omitempty" yaml:"includeDockerfile,omitempty"` // Include Dockerfile
	Required      bool            `json:"required,omitempty" yaml:"required,omitempty"`                   // Fail if provenance generation fails
	Builder       *BuilderConfig  `json:"builder,omitempty" yaml:"builder,omitempty"`
	Metadata      *MetadataConfig `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// BuilderConfig configures builder identification
type BuilderConfig struct {
	ID string `json:"id,omitempty" yaml:"id,omitempty"` // Auto-detected from CI if not specified
}

// MetadataConfig configures metadata collection
type MetadataConfig struct {
	IncludeEnv       bool `json:"includeEnv,omitempty" yaml:"includeEnv,omitempty"`             // Include environment variables
	IncludeMaterials bool `json:"includeMaterials,omitempty" yaml:"includeMaterials,omitempty"` // Include build materials
}

// ScanConfig configures vulnerability scanning
type ScanConfig struct {
	Enabled  bool             `json:"enabled" yaml:"enabled"`
	Tools    []ScanToolConfig `json:"tools,omitempty" yaml:"tools,omitempty"`
	FailOn   Severity         `json:"failOn,omitempty" yaml:"failOn,omitempty"`     // Fail on this severity or higher
	WarnOn   Severity         `json:"warnOn,omitempty" yaml:"warnOn,omitempty"`     // Warn on this severity or higher
	Required bool             `json:"required,omitempty" yaml:"required,omitempty"` // Fail if scan fails
}

// ScanToolConfig configures a specific scanning tool
type ScanToolConfig struct {
	Name     string   `json:"name" yaml:"name"`                             // grype, trivy
	Enabled  bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`   // Enable this tool
	Required bool     `json:"required,omitempty" yaml:"required,omitempty"` // Fail if this tool fails
	FailOn   Severity `json:"failOn,omitempty" yaml:"failOn,omitempty"`     // Tool-specific failOn
	WarnOn   Severity `json:"warnOn,omitempty" yaml:"warnOn,omitempty"`     // Tool-specific warnOn
}

// Severity represents vulnerability severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityNone     Severity = "" // No severity filtering
)

// Validate validates the security configuration
func (c *SecurityConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate signing config
	if c.Signing != nil && c.Signing.Enabled {
		if err := c.Signing.Validate(); err != nil {
			return fmt.Errorf("signing config validation failed: %w", err)
		}
	}

	// Validate SBOM config
	if c.SBOM != nil && c.SBOM.Enabled {
		if err := c.SBOM.Validate(); err != nil {
			return fmt.Errorf("sbom config validation failed: %w", err)
		}
	}

	// Validate provenance config
	if c.Provenance != nil && c.Provenance.Enabled {
		if err := c.Provenance.Validate(); err != nil {
			return fmt.Errorf("provenance config validation failed: %w", err)
		}
	}

	// Validate scan config
	if c.Scan != nil && c.Scan.Enabled {
		if err := c.Scan.Validate(); err != nil {
			return fmt.Errorf("scan config validation failed: %w", err)
		}
	}

	return nil
}

// Validate validates SBOM configuration
func (c *SBOMConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate format
	validFormats := []string{
		"cyclonedx-json",
		"cyclonedx-xml",
		"spdx-json",
		"spdx-tag-value",
		"syft-json",
	}

	if c.Format != "" {
		valid := false
		for _, f := range validFormats {
			if c.Format == f {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid sbom.format: %s (valid: %v)", c.Format, validFormats)
		}
	}

	// Validate generator
	validGenerators := []string{"syft"}
	if c.Generator != "" {
		valid := false
		for _, g := range validGenerators {
			if c.Generator == g {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid sbom.generator: %s (valid: %v)", c.Generator, validGenerators)
		}
	}

	return nil
}

// Validate validates provenance configuration
func (c *ProvenanceConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate format
	validFormats := []string{"slsa-v1.0", "slsa-v0.2"}
	if c.Format != "" {
		valid := false
		for _, f := range validFormats {
			if c.Format == f {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid provenance.format: %s (valid: %v)", c.Format, validFormats)
		}
	}

	return nil
}

// Validate validates scan configuration
func (c *ScanConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate failOn severity
	if c.FailOn != "" {
		if err := c.FailOn.Validate(); err != nil {
			return fmt.Errorf("invalid scan.failOn: %w", err)
		}
	}

	// Validate warnOn severity
	if c.WarnOn != "" {
		if err := c.WarnOn.Validate(); err != nil {
			return fmt.Errorf("invalid scan.warnOn: %w", err)
		}
	}

	// Validate tools
	if len(c.Tools) == 0 {
		return fmt.Errorf("scan.tools is required when scanning is enabled")
	}

	for i, tool := range c.Tools {
		if err := tool.Validate(); err != nil {
			return fmt.Errorf("scan.tools[%d] validation failed: %w", i, err)
		}
	}

	return nil
}

// Validate validates scan tool configuration
func (c *ScanToolConfig) Validate() error {
	// Validate tool name
	validTools := []string{"grype", "trivy"}
	valid := false
	for _, t := range validTools {
		if c.Name == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid tool name: %s (valid: %v)", c.Name, validTools)
	}

	// Validate failOn severity
	if c.FailOn != "" {
		if err := c.FailOn.Validate(); err != nil {
			return fmt.Errorf("invalid failOn: %w", err)
		}
	}

	// Validate warnOn severity
	if c.WarnOn != "" {
		if err := c.WarnOn.Validate(); err != nil {
			return fmt.Errorf("invalid warnOn: %w", err)
		}
	}

	return nil
}

// Validate validates severity level
func (s Severity) Validate() error {
	validSeverities := []Severity{
		SeverityCritical,
		SeverityHigh,
		SeverityMedium,
		SeverityLow,
		SeverityNone,
	}

	for _, v := range validSeverities {
		if s == v {
			return nil
		}
	}

	return fmt.Errorf("invalid severity: %s (valid: critical, high, medium, low)", s)
}

// IsAtLeast returns true if this severity is at least as severe as the given severity
func (s Severity) IsAtLeast(other Severity) bool {
	severityOrder := map[Severity]int{
		SeverityCritical: 4,
		SeverityHigh:     3,
		SeverityMedium:   2,
		SeverityLow:      1,
		SeverityNone:     0,
	}

	return severityOrder[s] >= severityOrder[other]
}

// DefaultSecurityConfig returns a default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		Enabled: false,
		Signing: &signing.Config{
			Enabled:  false,
			Keyless:  true,
			Required: false,
		},
		SBOM: &SBOMConfig{
			Enabled:   false,
			Format:    "cyclonedx-json",
			Generator: "syft",
			Output: &OutputConfig{
				Registry: true,
			},
			Attach: &AttachConfig{
				Enabled: true,
				Sign:    true,
			},
			Required: false,
		},
		Provenance: &ProvenanceConfig{
			Enabled:    false,
			Format:     "slsa-v1.0",
			IncludeGit: true,
			Required:   false,
			Metadata: &MetadataConfig{
				IncludeEnv:       false,
				IncludeMaterials: true,
			},
		},
		Scan: &ScanConfig{
			Enabled:  false,
			FailOn:   SeverityCritical,
			Required: false,
			Tools: []ScanToolConfig{
				{
					Name:     "grype",
					Enabled:  true,
					Required: true,
					FailOn:   SeverityCritical,
				},
			},
		},
	}
}
