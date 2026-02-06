//go:build integration
// +build integration

package security

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/simple-container-com/api/pkg/security/signing"
	"github.com/simple-container-com/api/pkg/security/tools"
)

// skipIfCosignNotInstalled skips the test if cosign is not installed
func skipIfCosignNotInstalled(t *testing.T) {
	t.Helper()
	installer := tools.NewToolInstaller()
	installed, err := installer.CheckInstalled("cosign")
	if err != nil || !installed {
		t.Skip("Skipping integration test: cosign not installed. Install from https://docs.sigstore.dev/cosign/installation/")
	}
}

// TestSecurityExecutorIntegration tests SecurityExecutor.ExecuteSigning with real cosign commands
func TestSecurityExecutorIntegration(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate test key pair
	password := "executor-test"
	privateKey, publicKey, err := signing.GenerateKeyPair(ctx, tempDir, password)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	tests := []struct {
		name      string
		config    *SecurityConfig
		imageRef  string
		wantError bool
		validate  func(t *testing.T, result *signing.SignResult, err error)
	}{
		{
			name: "Signing disabled",
			config: &SecurityConfig{
				Enabled: false,
			},
			imageRef:  "test-image:latest",
			wantError: false,
			validate: func(t *testing.T, result *signing.SignResult, err error) {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if result != nil {
					t.Error("Expected nil result when signing disabled")
				}
			},
		},
		{
			name: "Valid key-based config (will fail due to non-existent image)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Required:   false, // fail-open
					Keyless:    false,
					PrivateKey: privateKey,
					PublicKey:  publicKey,
					Password:   password,
					Timeout:    "30s",
				},
			},
			imageRef:  "test.registry.io/test:latest",
			wantError: false, // fail-open, so no error
			validate: func(t *testing.T, result *signing.SignResult, err error) {
				// With fail-open, should return nil error and nil result
				if err != nil {
					t.Errorf("Expected no error with fail-open, got: %v", err)
				}
			},
		},
		{
			name: "Required signing fails (fail-closed)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Required:   true, // fail-closed
					Keyless:    false,
					PrivateKey: privateKey,
					Password:   password,
					Timeout:    "10s",
				},
			},
			imageRef:  "nonexistent.registry/test:latest",
			wantError: true, // fail-closed, so error expected
			validate: func(t *testing.T, result *signing.SignResult, err error) {
				if err == nil {
					t.Error("Expected error with fail-closed on non-existent image")
				} else {
					t.Logf("Got expected error: %v", err)
				}
			},
		},
		{
			name: "Invalid config (missing private key)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Required:   false,
					Keyless:    false,
					PrivateKey: "", // Missing!
					Timeout:    "30s",
				},
			},
			imageRef:  "test:latest",
			wantError: false, // fail-open
			validate: func(t *testing.T, result *signing.SignResult, err error) {
				// Should log warning and continue with nil result
				if result != nil {
					t.Error("Expected nil result on validation failure")
				}
			},
		},
		{
			name: "Keyless config without OIDC token",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Required:       false,
					Keyless:        true,
					OIDCIssuer:     "https://token.actions.githubusercontent.com",
					IdentityRegexp: "^https://github.com/.*$",
					Timeout:        "30s",
				},
			},
			imageRef:  "test:latest",
			wantError: false, // fail-open
			validate: func(t *testing.T, result *signing.SignResult, err error) {
				// Without OIDC token, should fail gracefully
				if result != nil {
					t.Error("Expected nil result without OIDC token")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}

			result, err := executor.ExecuteSigning(ctx, tt.imageRef)

			if (err != nil) != tt.wantError {
				t.Errorf("ExecuteSigning() error = %v, wantError %v", err, tt.wantError)
			}

			if tt.validate != nil {
				tt.validate(t, result, err)
			}
		})
	}
}

// TestSecurityExecutorValidateConfig tests config validation
func TestSecurityExecutorValidateConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		config    *SecurityConfig
		wantError bool
	}{
		{
			name:      "Nil config (valid)",
			config:    nil,
			wantError: false,
		},
		{
			name: "Disabled config (valid)",
			config: &SecurityConfig{
				Enabled: false,
			},
			wantError: false,
		},
		{
			name: "Valid key-based config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Keyless:    false,
					PrivateKey: "/path/to/key.pem",
					PublicKey:  "/path/to/key.pub",
				},
			},
			wantError: false,
		},
		{
			name: "Valid keyless config",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Keyless:        true,
					OIDCIssuer:     "https://token.actions.githubusercontent.com",
					IdentityRegexp: "^https://github.com/.*$",
				},
			},
			wantError: false,
		},
		{
			name: "Invalid key-based config (no private key)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Keyless:    false,
					PrivateKey: "",
				},
			},
			wantError: true,
		},
		{
			name: "Invalid keyless config (no OIDC issuer)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Keyless:        true,
					IdentityRegexp: "^https://github.com/.*$",
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}

			err = executor.ValidateConfig()
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateConfig() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestSecurityExecutorWithRealKeys tests executor with real generated keys
func TestSecurityExecutorWithRealKeys(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate real test keys
	password := "real-key-test"
	privateKey, publicKey, err := signing.GenerateKeyPair(ctx, tempDir, password)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	t.Logf("Generated real keys: private=%s, public=%s", privateKey, publicKey)

	// Create executor with real keys
	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:    true,
			Required:   false, // fail-open for testing
			Keyless:    false,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
			Password:   password,
			Timeout:    "30s",
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Validate config
	if err := executor.ValidateConfig(); err != nil {
		t.Errorf("ValidateConfig failed: %v", err)
	}

	// Try to sign (will fail due to non-existent image, but validates the flow)
	testImage := "test.registry.io/executor-test:v1"
	result, err := executor.ExecuteSigning(ctx, testImage)
	// With fail-open, should not error
	if err != nil {
		t.Errorf("Expected no error with fail-open, got: %v", err)
	}

	// Result will be nil because signing failed (image doesn't exist)
	if result != nil {
		t.Logf("Unexpected result: %+v", result)
	}

	t.Log("Executor test with real keys completed")
}

// TestSecurityExecutorFailOpenLogging tests that warnings are logged
func TestSecurityExecutorFailOpenLogging(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()

	// Create config with invalid private key path
	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:    true,
			Required:   false, // fail-open
			Keyless:    false,
			PrivateKey: "/nonexistent/key.pem",
			Password:   "test",
			Timeout:    "5s",
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute signing (should log warning but not fail)
	testImage := "test:latest"
	result, err := executor.ExecuteSigning(ctx, testImage)
	// Should not return error (fail-open)
	if err != nil {
		t.Errorf("Expected no error with fail-open, got: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result on signing failure")
	}

	t.Log("Fail-open logging test completed (check logs for warnings)")
}

// TestSecurityExecutorOIDCToken tests OIDC token handling
func TestSecurityExecutorOIDCToken(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()

	// Test with OIDC token from environment
	oidcToken := os.Getenv("TEST_OIDC_TOKEN")
	if oidcToken == "" {
		t.Skip("Skipping OIDC test: TEST_OIDC_TOKEN not set")
	}

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:        true,
			Required:       false,
			Keyless:        true,
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
			IdentityRegexp: "^https://github.com/.*$",
			Timeout:        "30s",
		},
	}

	// Create executor with OIDC token in context
	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Set OIDC token in execution context
	executor.Context.OIDCToken = oidcToken

	// Try signing
	testImage := "test.registry.io/oidc-test:latest"
	result, err := executor.ExecuteSigning(ctx, testImage)
	// Will likely fail due to non-existent image, but validates OIDC flow
	if err != nil {
		t.Logf("Expected error with non-existent image: %v", err)
	}
	if result != nil {
		t.Logf("Unexpected success with result: %+v", result)
	}

	t.Log("OIDC token test completed")
}

// TestSecurityExecutorTimeout tests timeout handling
func TestSecurityExecutorTimeout(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate keys
	privateKey, _, err := signing.GenerateKeyPair(ctx, tempDir, "test")
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create config with very short timeout
	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:    true,
			Required:   false,
			Keyless:    false,
			PrivateKey: privateKey,
			Password:   "test",
			Timeout:    "1ns", // Very short timeout
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Try signing (should timeout)
	result, err := executor.ExecuteSigning(ctx, "test:latest")
	// With fail-open, should not return error
	if err != nil {
		t.Logf("Error with timeout (expected): %v", err)
	}
	if result != nil {
		t.Error("Expected nil result on timeout")
	}

	t.Log("Timeout handling test completed")
}

// TestSecurityExecutorContextCancellation tests context cancellation
func TestSecurityExecutorContextCancellation(t *testing.T) {
	skipIfCosignNotInstalled(t)

	tempDir := t.TempDir()

	// Generate keys
	ctx := context.Background()
	privateKey, _, err := signing.GenerateKeyPair(ctx, tempDir, "test")
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	config := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:    true,
			Required:   false,
			Keyless:    false,
			PrivateKey: privateKey,
			Password:   "test",
			Timeout:    "60s",
		},
	}

	executor, err := NewSecurityExecutor(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create cancellable context
	ctxCancel, cancel := context.WithCancel(ctx)

	// Cancel immediately
	cancel()

	// Try signing with cancelled context
	result, err := executor.ExecuteSigning(ctxCancel, "test:latest")

	// Should handle cancellation gracefully
	if result != nil {
		t.Error("Expected nil result with cancelled context")
	}

	t.Logf("Context cancellation test completed: err=%v", err)
}

// TestSecurityExecutorConfigurationPrecedence tests config validation order
func TestSecurityExecutorConfigurationPrecedence(t *testing.T) {
	ctx := context.Background()

	// Test that nil config is handled
	executor1, err := NewSecurityExecutor(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to create executor with nil config: %v", err)
	}
	if executor1.Config == nil {
		t.Error("Expected non-nil config (default)")
	}
	if executor1.Config.Enabled {
		t.Error("Expected default config to be disabled")
	}

	// Test that empty config works
	executor2, err := NewSecurityExecutor(ctx, &SecurityConfig{})
	if err != nil {
		t.Fatalf("Failed to create executor with empty config: %v", err)
	}
	if executor2.Config.Enabled {
		t.Error("Expected empty config to be disabled")
	}

	t.Log("Configuration precedence test completed")
}

// TestSecurityExecutorErrorMessages tests error message quality
func TestSecurityExecutorErrorMessages(t *testing.T) {
	skipIfCosignNotInstalled(t)

	ctx := context.Background()

	tests := []struct {
		name          string
		config        *SecurityConfig
		imageRef      string
		checkErrorMsg func(t *testing.T, err error)
	}{
		{
			name: "Missing private key (fail-closed)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:    true,
					Required:   true, // fail-closed for error message
					Keyless:    false,
					PrivateKey: "",
				},
			},
			imageRef: "test:latest",
			checkErrorMsg: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for missing private key")
					return
				}
				errMsg := err.Error()
				if !strings.Contains(errMsg, "private_key") && !strings.Contains(errMsg, "private key") {
					t.Errorf("Error message should mention private key: %v", err)
				}
			},
		},
		{
			name: "Missing OIDC issuer (fail-closed)",
			config: &SecurityConfig{
				Enabled: true,
				Signing: &signing.Config{
					Enabled:        true,
					Required:       true,
					Keyless:        true,
					IdentityRegexp: "^https://github.com/.*$",
					OIDCIssuer:     "", // Missing
				},
			},
			imageRef: "test:latest",
			checkErrorMsg: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected error for missing OIDC issuer")
					return
				}
				errMsg := err.Error()
				if !strings.Contains(errMsg, "oidc") && !strings.Contains(errMsg, "OIDC") {
					t.Errorf("Error message should mention OIDC: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewSecurityExecutor(ctx, tt.config)
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}

			_, err = executor.ExecuteSigning(ctx, tt.imageRef)
			tt.checkErrorMsg(t, err)
		})
	}
}
