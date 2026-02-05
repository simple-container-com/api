package security

import (
	"context"
	"fmt"
	"time"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/security/tools"
)

// Executor orchestrates security operations
type Executor interface {
	// Execute runs all enabled security operations
	Execute(ctx context.Context, image ImageReference) (*SecurityResult, error)

	// ValidateConfig validates security configuration
	ValidateConfig(config *api.SecurityDescriptor) error
}

// executor implements the Executor interface
type executor struct {
	config  *api.SecurityDescriptor
	context *ExecutionContext
	logger  logger.Logger
	cache   *Cache
	tools   *tools.Installer
}

// NewExecutor creates a new security executor
func NewExecutor(
	config *api.SecurityDescriptor,
	execContext *ExecutionContext,
	log logger.Logger,
) (Executor, error) {
	if config == nil {
		return nil, fmt.Errorf("security config is nil")
	}

	if log == nil {
		return nil, fmt.Errorf("logger is nil")
	}

	// Create cache with default settings
	cache, err := NewCache("", 12*time.Hour)
	if err != nil {
		log.Warn(context.Background(), "Failed to create cache: %v", err)
		cache = nil // Continue without cache
	}

	// Create execution context if not provided
	if execContext == nil {
		execContext, err = NewExecutionContext()
		if err != nil {
			return nil, fmt.Errorf("failed to create execution context: %w", err)
		}
	}

	return &executor{
		config:  config,
		context: execContext,
		logger:  log,
		cache:   cache,
		tools:   tools.NewInstaller(log),
	}, nil
}

// Execute implements Executor
func (e *executor) Execute(ctx context.Context, image ImageReference) (*SecurityResult, error) {
	result := &SecurityResult{
		Image:     image,
		StartedAt: time.Now(),
		Errors:    []error{},
		Warnings:  []string{},
	}

	e.logger.Info(ctx, "Starting security operations for image: %s", image.String())

	// 1. Validate configuration
	if err := e.ValidateConfig(e.config); err != nil {
		return result, fmt.Errorf("invalid security configuration: %w", err)
	}

	// 2. Check tool availability
	if err := e.tools.CheckAllTools(ctx, e.config); err != nil {
		return result, fmt.Errorf("tool check failed: %w", err)
	}

	// 3. Execute scanning (fail-fast if configured)
	if e.config.Scan != nil && e.config.Scan.Enabled {
		e.logger.Info(ctx, "Vulnerability scanning enabled")
		// Scanning implementation will be added in Phase 4
		result.Warnings = append(result.Warnings, "Scanning not yet implemented (Phase 4)")
	}

	// 4. Execute signing
	if e.config.Signing != nil && e.config.Signing.Enabled {
		e.logger.Info(ctx, "Image signing enabled")
		// Signing implementation will be added in Phase 2
		result.Warnings = append(result.Warnings, "Signing not yet implemented (Phase 2)")
	}

	// 5. Execute SBOM generation
	if e.config.SBOM != nil && e.config.SBOM.Enabled {
		e.logger.Info(ctx, "SBOM generation enabled")
		// SBOM implementation will be added in Phase 3
		result.Warnings = append(result.Warnings, "SBOM generation not yet implemented (Phase 3)")
	}

	// 6. Execute provenance generation
	if e.config.Provenance != nil && e.config.Provenance.Enabled {
		e.logger.Info(ctx, "Provenance generation enabled")
		// Provenance implementation will be added in Phase 4
		result.Warnings = append(result.Warnings, "Provenance generation not yet implemented (Phase 4)")
	}

	result.FinishedAt = time.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)

	e.logger.Info(ctx, "Security operations completed in %v", result.Duration)

	return result, nil
}

// ValidateConfig implements Executor
func (e *executor) ValidateConfig(config *api.SecurityDescriptor) error {
	if config == nil {
		return &ConfigurationError{
			Field:   "security",
			Message: "security configuration is nil",
		}
	}

	// Validate signing config
	if config.Signing != nil && config.Signing.Enabled {
		if err := e.validateSigningConfig(config.Signing); err != nil {
			return err
		}
	}

	// Validate SBOM config
	if config.SBOM != nil && config.SBOM.Enabled {
		if err := e.validateSBOMConfig(config.SBOM); err != nil {
			return err
		}
	}

	// Validate provenance config
	if config.Provenance != nil && config.Provenance.Enabled {
		if err := e.validateProvenanceConfig(config.Provenance); err != nil {
			return err
		}
	}

	// Validate scan config
	if config.Scan != nil && config.Scan.Enabled {
		if err := e.validateScanConfig(config.Scan); err != nil {
			return err
		}
	}

	return nil
}

// validateSigningConfig validates signing configuration
func (e *executor) validateSigningConfig(config *api.SigningConfig) error {
	// If not keyless, require private key
	if !config.Keyless {
		if config.PrivateKey == "" {
			return &ConfigurationError{
				Field:   "signing.privateKey",
				Message: "private key required for key-based signing",
			}
		}
	}

	// Validate provider
	if config.Provider != "" && config.Provider != "sigstore" {
		return &ConfigurationError{
			Field:   "signing.provider",
			Message: fmt.Sprintf("unsupported signing provider: %s", config.Provider),
		}
	}

	// Validate verification config
	if config.Verify != nil && config.Verify.Enabled {
		if config.Keyless && config.Verify.OIDCIssuer == "" {
			return &ConfigurationError{
				Field:   "signing.verify.oidcIssuer",
				Message: "OIDC issuer required for keyless signature verification",
			}
		}
	}

	return nil
}

// validateSBOMConfig validates SBOM configuration
func (e *executor) validateSBOMConfig(config *api.SBOMConfig) error {
	// Validate format
	validFormats := []string{"cyclonedx-json", "cyclonedx-xml", "spdx-json", "spdx-tag-value", "syft-json"}
	if config.Format != "" {
		valid := false
		for _, f := range validFormats {
			if config.Format == f {
				valid = true
				break
			}
		}
		if !valid {
			return &ConfigurationError{
				Field:   "sbom.format",
				Message: fmt.Sprintf("unsupported SBOM format: %s (valid: %v)", config.Format, validFormats),
			}
		}
	}

	// Validate generator
	if config.Generator != "" && config.Generator != "syft" {
		return &ConfigurationError{
			Field:   "sbom.generator",
			Message: fmt.Sprintf("unsupported SBOM generator: %s (only 'syft' is supported)", config.Generator),
		}
	}

	return nil
}

// validateProvenanceConfig validates provenance configuration
func (e *executor) validateProvenanceConfig(config *api.ProvenanceConfig) error {
	// Validate version
	if config.Version != "" && config.Version != "1.0" {
		return &ConfigurationError{
			Field:   "provenance.version",
			Message: fmt.Sprintf("unsupported provenance version: %s (only '1.0' is supported)", config.Version),
		}
	}

	return nil
}

// validateScanConfig validates scan configuration
func (e *executor) validateScanConfig(config *api.ScanConfig) error {
	if len(config.Tools) == 0 {
		return &ConfigurationError{
			Field:   "scan.tools",
			Message: "at least one scanning tool must be configured",
		}
	}

	// Validate each tool config
	for i, tool := range config.Tools {
		if tool.Name == "" {
			return &ConfigurationError{
				Field:   fmt.Sprintf("scan.tools[%d].name", i),
				Message: "tool name is required",
			}
		}

		// Check if tool is supported
		if _, exists := tools.ToolRegistry[tool.Name]; !exists {
			return &ConfigurationError{
				Field:   fmt.Sprintf("scan.tools[%d].name", i),
				Message: fmt.Sprintf("unsupported scanning tool: %s", tool.Name),
			}
		}

		// Validate severity levels
		if tool.FailOn != "" {
			if !e.isValidSeverity(tool.FailOn) {
				return &ConfigurationError{
					Field:   fmt.Sprintf("scan.tools[%d].failOn", i),
					Message: fmt.Sprintf("invalid severity level: %s", tool.FailOn),
				}
			}
		}

		if tool.WarnOn != "" {
			if !e.isValidSeverity(tool.WarnOn) {
				return &ConfigurationError{
					Field:   fmt.Sprintf("scan.tools[%d].warnOn", i),
					Message: fmt.Sprintf("invalid severity level: %s", tool.WarnOn),
				}
			}
		}
	}

	return nil
}

// isValidSeverity checks if severity level is valid
func (e *executor) isValidSeverity(severity api.Severity) bool {
	validSeverities := []api.Severity{
		api.SeverityCritical,
		api.SeverityHigh,
		api.SeverityMedium,
		api.SeverityLow,
	}

	for _, valid := range validSeverities {
		if severity == valid {
			return true
		}
	}

	return false
}
