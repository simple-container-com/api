package scan

import "fmt"

// Severity represents vulnerability severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityUnknown  Severity = "unknown"
)

// ValidSeverities lists all valid severity levels
var ValidSeverities = []Severity{
	SeverityCritical,
	SeverityHigh,
	SeverityMedium,
	SeverityLow,
	SeverityUnknown,
}

// ScanTool represents a vulnerability scanning tool
type ScanTool string

const (
	ScanToolGrype ScanTool = "grype"
	ScanToolTrivy ScanTool = "trivy"
	ScanToolAll   ScanTool = "all"
)

// Config represents scanning configuration
type Config struct {
	Enabled  bool          `json:"enabled" yaml:"enabled"`
	Tools    []ScanTool    `json:"tools" yaml:"tools"`
	FailOn   Severity      `json:"failOn" yaml:"failOn"`
	WarnOn   Severity      `json:"warnOn" yaml:"warnOn"`
	Required bool          `json:"required" yaml:"required"`
	Output   *OutputConfig `json:"output,omitempty" yaml:"output,omitempty"`
	Cache    *CacheConfig  `json:"cache,omitempty" yaml:"cache,omitempty"`
}

// OutputConfig configures scan output
type OutputConfig struct {
	Local    string `json:"local,omitempty" yaml:"local,omitempty"`
	Registry bool   `json:"registry" yaml:"registry"`
}

// CacheConfig configures scan result caching
type CacheConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	TTL     int  `json:"ttl" yaml:"ttl"` // TTL in hours
}

// DefaultConfig returns default scanning configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:  true,
		Tools:    []ScanTool{ScanToolGrype},
		FailOn:   SeverityCritical,
		WarnOn:   SeverityHigh,
		Required: true,
		Output:   &OutputConfig{},
		Cache: &CacheConfig{
			Enabled: true,
			TTL:     6, // 6 hours
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if len(c.Tools) == 0 {
		return fmt.Errorf("at least one scanning tool must be specified")
	}

	// Validate tools
	for _, tool := range c.Tools {
		if tool != ScanToolGrype && tool != ScanToolTrivy && tool != ScanToolAll {
			return fmt.Errorf("invalid scan tool: %s (must be grype, trivy, or all)", tool)
		}
	}

	// Validate failOn severity
	if c.FailOn != "" && !isValidSeverity(c.FailOn) {
		return fmt.Errorf("invalid failOn severity: %s", c.FailOn)
	}

	// Validate warnOn severity
	if c.WarnOn != "" && !isValidSeverity(c.WarnOn) {
		return fmt.Errorf("invalid warnOn severity: %s", c.WarnOn)
	}

	return nil
}

// isValidSeverity checks if a severity level is valid
func isValidSeverity(s Severity) bool {
	for _, valid := range ValidSeverities {
		if s == valid {
			return true
		}
	}
	return false
}

// ShouldCache returns true if caching is enabled
func (c *Config) ShouldCache() bool {
	return c.Cache != nil && c.Cache.Enabled
}

// ShouldSaveLocal returns true if local output is configured
func (c *Config) ShouldSaveLocal() bool {
	return c.Output != nil && c.Output.Local != ""
}
