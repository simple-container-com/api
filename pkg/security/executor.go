package security

import (
	"context"
	"fmt"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// SecurityExecutor orchestrates all security operations for container images
type SecurityExecutor struct {
	Context *ExecutionContext
	Config  *SecurityConfig
}

// SecurityConfig contains configuration for all security operations
type SecurityConfig struct {
	Enabled bool
	Signing *signing.Config
	// Future: SBOM, Provenance, Scanning configs
}

// NewSecurityExecutor creates a new security executor
func NewSecurityExecutor(ctx context.Context, config *SecurityConfig) (*SecurityExecutor, error) {
	if config == nil {
		config = &SecurityConfig{Enabled: false}
	}

	execCtx, err := NewExecutionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating execution context: %w", err)
	}

	return &SecurityExecutor{
		Context: execCtx,
		Config:  config,
	}, nil
}

// ExecuteSigning performs signing operations on the image
func (e *SecurityExecutor) ExecuteSigning(ctx context.Context, imageRef string) (*signing.SignResult, error) {
	if !e.Config.Enabled || e.Config.Signing == nil || !e.Config.Signing.Enabled {
		return nil, nil // Signing disabled
	}

	// Validate signing configuration
	if err := e.Config.Signing.Validate(); err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("signing validation failed: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: signing validation failed, skipping: %v\n", err)
		return nil, nil
	}

	// Create signer
	signer, err := e.Config.Signing.CreateSigner(e.Context.OIDCToken)
	if err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("creating signer: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: failed to create signer, skipping: %v\n", err)
		return nil, nil
	}

	// Execute signing
	result, err := signer.Sign(ctx, imageRef)
	if err != nil {
		if e.Config.Signing.Required {
			return nil, fmt.Errorf("signing image: %w", err)
		}
		// Fail-open: log warning and continue
		fmt.Printf("Warning: signing failed, continuing: %v\n", err)
		return nil, nil
	}

	return result, nil
}

// ValidateConfig validates the security configuration
func (e *SecurityExecutor) ValidateConfig() error {
	if !e.Config.Enabled {
		return nil
	}

	if e.Config.Signing != nil && e.Config.Signing.Enabled {
		if err := e.Config.Signing.Validate(); err != nil {
			return fmt.Errorf("signing config validation failed: %w", err)
		}
	}

	return nil
}
