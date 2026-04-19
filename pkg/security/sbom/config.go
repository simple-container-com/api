package sbom

import (
	"fmt"
)

// Config represents SBOM generation configuration
type Config struct {
	// Enabled indicates if SBOM generation is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Format specifies the SBOM format (cyclonedx-json, spdx-json, etc.)
	Format Format `json:"format,omitempty" yaml:"format,omitempty"`

	// Generator specifies the tool to use (only "syft" supported currently)
	Generator string `json:"generator,omitempty" yaml:"generator,omitempty"`

	// Output specifies where to save the SBOM
	Output *OutputConfig `json:"output,omitempty" yaml:"output,omitempty"`

	// Attach indicates if SBOM should be attached as attestation
	Attach bool `json:"attach,omitempty" yaml:"attach,omitempty"`

	// Required indicates if SBOM generation failure should fail the build
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// CacheEnabled indicates if caching should be used
	CacheEnabled bool `json:"cacheEnabled,omitempty" yaml:"cacheEnabled,omitempty"`
}

// OutputConfig specifies SBOM output configuration
type OutputConfig struct {
	// Local file path to save SBOM
	Local string `json:"local,omitempty" yaml:"local,omitempty"`

	// Registry indicates if SBOM should be pushed to registry as attestation
	Registry bool `json:"registry,omitempty" yaml:"registry,omitempty"`
}

// DefaultConfig returns the default SBOM configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:      false,
		Format:       FormatCycloneDXJSON,
		Generator:    "syft",
		CacheEnabled: true,
		Output: &OutputConfig{
			Local:    "",
			Registry: false,
		},
		Attach:   false,
		Required: false,
	}
}

// Validate validates the SBOM configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Validate format
	if c.Format == "" {
		c.Format = FormatCycloneDXJSON
	}
	if !c.Format.IsValid() {
		return fmt.Errorf("invalid SBOM format: %s (supported: %v)", c.Format, AllFormatStrings())
	}

	// Validate generator
	if c.Generator == "" {
		c.Generator = "syft"
	}
	if c.Generator != "syft" {
		return fmt.Errorf("invalid SBOM generator: %s (only 'syft' is supported)", c.Generator)
	}

	// Validate output config
	if c.Output == nil {
		c.Output = &OutputConfig{}
	}

	return nil
}

// ShouldCache returns true if caching should be used
func (c *Config) ShouldCache() bool {
	return c.CacheEnabled
}

// ShouldSaveLocal returns true if SBOM should be saved locally
func (c *Config) ShouldSaveLocal() bool {
	return c.Output != nil && c.Output.Local != ""
}

// ShouldAttach returns true if SBOM should be attached as attestation
func (c *Config) ShouldAttach() bool {
	return c.Attach || (c.Output != nil && c.Output.Registry)
}

// IsRequired returns true if SBOM generation is required (fail-closed)
func (c *Config) IsRequired() bool {
	return c.Required
}
