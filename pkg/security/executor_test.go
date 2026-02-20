package security

import (
	"context"
	"testing"

	"github.com/simple-container-com/api/pkg/security/signing"
)

func TestNewSecurityExecutor(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "disabled config",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "enabled config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: true,
					Keyless: true,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSecurityExecutor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && executor == nil {
				t.Error("NewSecurityExecutor() returned nil without error")
			}
		})
	}
}

func TestSecurityExecutor_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *SecurityConfig
		wantErr bool
	}{
		{
			name: "disabled config",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid keyless signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Keyless:        true,
					OIDCIssuer:     "https://token.actions.githubusercontent.com",
					IdentityRegexp: "^https://github.com/org/.*$",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: true,
					Keyless: true,
					// Missing required OIDCIssuer
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("NewSecurityExecutor() failed: %v", err)
			}

			err = executor.ValidateConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_Disabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		config *SecurityConfig
	}{
		{
			name: "security disabled",
			config: &SecurityConfig{
				Enabled: false,
			},
		},
		{
			name: "signing disabled",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled: false,
				},
			},
		},
		{
			name: "nil signing config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("NewSecurityExecutor() failed: %v", err)
			}

			result, err := executor.ExecuteSigning(ctx, "test-image:latest")
			if err != nil {
				t.Errorf("ExecuteSigning() returned error for disabled config: %v", err)
			}
			if result != nil {
				t.Error("ExecuteSigning() should return nil for disabled config")
			}
		})
	}
}

func TestSecurityExecutor_ExecuteSigning_FailOpen(t *testing.T) {
	ctx := context.Background()

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:  true,
			Required: false, // Fail-open
			Keyless:  true,
			// Invalid config: missing OIDCIssuer
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("NewSecurityExecutor() failed: %v", err)
	}

	// Should not error because fail-open is enabled
	result, err := executor.ExecuteSigning(ctx, "test-image:latest")
	if err != nil {
		t.Errorf("ExecuteSigning() with fail-open should not error: %v", err)
	}
	if result != nil {
		t.Error("ExecuteSigning() should return nil when validation fails with fail-open")
	}
}

func TestSecurityExecutor_ExecuteSigning_FailClosed(t *testing.T) {
	ctx := context.Background()

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:  true,
			Required: true, // Fail-closed
			Keyless:  true,
			// Invalid config: missing OIDCIssuer
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("NewSecurityExecutor() failed: %v", err)
	}

	// Should error because fail-closed is enabled
	_, err = executor.ExecuteSigning(ctx, "test-image:latest")
	if err == nil {
		t.Error("ExecuteSigning() with fail-closed should error on invalid config")
	}
}
