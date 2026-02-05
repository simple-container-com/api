package security

import (
	"errors"
	"fmt"
)

var (
	// ErrToolNotFound indicates a required security tool is not installed
	ErrToolNotFound = errors.New("security tool not found")

	// ErrToolVersionMismatch indicates tool version doesn't meet requirements
	ErrToolVersionMismatch = errors.New("tool version mismatch")

	// ErrInvalidConfiguration indicates security configuration is invalid
	ErrInvalidConfiguration = errors.New("invalid security configuration")

	// ErrCriticalVulnerabilities indicates critical vulnerabilities were found
	ErrCriticalVulnerabilities = errors.New("critical vulnerabilities found")

	// ErrSigningFailed indicates image signing failed
	ErrSigningFailed = errors.New("image signing failed")

	// ErrSBOMGenerationFailed indicates SBOM generation failed
	ErrSBOMGenerationFailed = errors.New("SBOM generation failed")

	// ErrProvenanceGenerationFailed indicates provenance generation failed
	ErrProvenanceGenerationFailed = errors.New("provenance generation failed")

	// ErrScanFailed indicates vulnerability scan failed
	ErrScanFailed = errors.New("vulnerability scan failed")

	// ErrVerificationFailed indicates signature verification failed
	ErrVerificationFailed = errors.New("signature verification failed")

	// ErrCacheError indicates cache operation failed
	ErrCacheError = errors.New("cache operation failed")

	// ErrInvalidImageReference indicates image reference is invalid
	ErrInvalidImageReference = errors.New("invalid image reference")

	// ErrOIDCTokenUnavailable indicates OIDC token is not available
	ErrOIDCTokenUnavailable = errors.New("OIDC token unavailable")

	// ErrRegistryAuthFailed indicates registry authentication failed
	ErrRegistryAuthFailed = errors.New("registry authentication failed")
)

// ToolNotFoundError wraps ErrToolNotFound with tool details
type ToolNotFoundError struct {
	ToolName   string
	MinVersion string
	InstallURL string
}

func (e *ToolNotFoundError) Error() string {
	if e.MinVersion != "" {
		return fmt.Sprintf("%s: %s (minimum version %s required). Install from: %s",
			ErrToolNotFound, e.ToolName, e.MinVersion, e.InstallURL)
	}
	return fmt.Sprintf("%s: %s. Install from: %s",
		ErrToolNotFound, e.ToolName, e.InstallURL)
}

func (e *ToolNotFoundError) Unwrap() error {
	return ErrToolNotFound
}

// ToolVersionMismatchError wraps ErrToolVersionMismatch with version details
type ToolVersionMismatchError struct {
	ToolName       string
	CurrentVersion string
	MinVersion     string
}

func (e *ToolVersionMismatchError) Error() string {
	return fmt.Sprintf("%s: %s version %s does not meet minimum %s",
		ErrToolVersionMismatch, e.ToolName, e.CurrentVersion, e.MinVersion)
}

func (e *ToolVersionMismatchError) Unwrap() error {
	return ErrToolVersionMismatch
}

// VulnerabilitiesFoundError wraps ErrCriticalVulnerabilities with details
type VulnerabilitiesFoundError struct {
	Critical int
	High     int
	Medium   int
	Low      int
	FailOn   string
}

func (e *VulnerabilitiesFoundError) Error() string {
	return fmt.Sprintf("%s: found %d critical, %d high, %d medium, %d low vulnerabilities (failOn: %s)",
		ErrCriticalVulnerabilities, e.Critical, e.High, e.Medium, e.Low, e.FailOn)
}

func (e *VulnerabilitiesFoundError) Unwrap() error {
	return ErrCriticalVulnerabilities
}

// ConfigurationError wraps ErrInvalidConfiguration with details
type ConfigurationError struct {
	Field   string
	Message string
}

func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ErrInvalidConfiguration, e.Field, e.Message)
}

func (e *ConfigurationError) Unwrap() error {
	return ErrInvalidConfiguration
}
