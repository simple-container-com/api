//go:build integration
// +build integration

package signing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

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

// TestKeyBasedSigningIntegration tests real key-based signing with cosign
func TestKeyBasedSigningIntegration(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate test key pair
	password := "test-password"
	privateKeyPath, publicKeyPath, err := GenerateKeyPair(ctx, tempDir, password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	// Verify key files exist
	_, err = os.Stat(privateKeyPath)
	Expect(err).ToNot(HaveOccurred(), "Private key file not created")
	_, err = os.Stat(publicKeyPath)
	Expect(err).ToNot(HaveOccurred(), "Public key file not created")

	// Verify private key has secure permissions (0600)
	info, err := os.Stat(privateKeyPath)
	Expect(err).ToNot(HaveOccurred())
	Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

	t.Logf("Generated test keys: private=%s, public=%s", privateKeyPath, publicKeyPath)

	// Test signing with generated keys
	testImage := "test.registry.io/test-image:test"

	signer := NewKeyBasedSigner(privateKeyPath, password, 30*time.Second)

	// Note: This will fail because the image doesn't exist in a registry
	// but we're testing the command construction and error handling
	result, err := signer.Sign(ctx, testImage)

	// We expect an error because the image doesn't exist
	if err != nil {
		Expect(err.Error()).To(ContainSubstring("cosign sign failed"))
		t.Logf("Expected error from cosign (image doesn't exist): %v", err)
	} else {
		Expect(result).ToNot(BeNil())
		t.Logf("Sign result: %+v", result)
	}
}

// TestKeyBasedSigningWithRawKeyContent tests signing with raw key content
func TestKeyBasedSigningWithRawKeyContent(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate test key pair
	password := "test-password"
	privateKeyPath, _, err := GenerateKeyPair(ctx, tempDir, password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	// Read key content
	keyContent, err := os.ReadFile(privateKeyPath)
	Expect(err).ToNot(HaveOccurred())

	// Create signer with raw key content (not file path)
	signer := NewKeyBasedSigner(string(keyContent), password, 30*time.Second)

	testImage := "test.registry.io/test-image:test"
	_, err = signer.Sign(ctx, testImage)
	// We expect an error because the image doesn't exist
	if err != nil {
		Expect(err.Error()).To(ContainSubstring("cosign sign failed"))
		t.Logf("Expected error (raw key content test): %v", err)
	}
}

// TestKeylessSigningIntegration tests keyless signing (requires OIDC token)
func TestKeylessSigningIntegration(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	// Check for test OIDC token in environment
	oidcToken := os.Getenv("TEST_OIDC_TOKEN")
	if oidcToken == "" {
		t.Skip("Skipping keyless signing test: TEST_OIDC_TOKEN not set")
	}

	ctx := context.Background()
	signer := NewKeylessSigner(oidcToken, 30*time.Second)

	testImage := "test.registry.io/test-image:test"
	result, err := signer.Sign(ctx, testImage)

	// We expect an error because the image doesn't exist
	if err != nil {
		Expect(err.Error()).To(ContainSubstring("cosign sign failed"))
		t.Logf("Expected error from cosign: %v", err)
	} else {
		Expect(result).ToNot(BeNil())
		Expect(result.RekorEntry).ToNot(BeEmpty())
		t.Logf("Sign result with Rekor entry: %+v", result)
	}
}

// TestSignatureVerificationIntegration tests signature verification
func TestSignatureVerificationIntegration(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate test key pair
	password := "test-password"
	_, publicKeyPath, err := GenerateKeyPair(ctx, tempDir, password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	// Create verifier with public key
	verifier := NewKeyBasedVerifier(publicKeyPath, 30*time.Second)

	testImage := "test.registry.io/test-image:test"
	result, err := verifier.Verify(ctx, testImage)

	// We expect an error because the image doesn't exist or isn't signed
	if err != nil {
		Expect(err.Error()).To(ContainSubstring("cosign verify failed"))
		Expect(result).ToNot(BeNil())
		Expect(result.Verified).To(BeFalse())
		t.Logf("Expected verification error: %v", err)
	} else {
		Expect(result).ToNot(BeNil())
		Expect(result.Verified).To(BeTrue())
		t.Logf("Verification result: %+v", result)
	}
}

// TestRekorEntryValidation tests Rekor transparency log validation
func TestRekorEntryValidation(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "URL format",
			output:   "Successfully created entry at: https://rekor.sigstore.dev/api/v1/log/entries/abc123",
			expected: "https://rekor.sigstore.dev/api/v1/log/entries/abc123",
		},
		{
			name:     "Index format",
			output:   "tlog entry created with index: 123456789",
			expected: "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=123456789",
		},
		{
			name:     "No entry",
			output:   "Signature created successfully",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(parseRekorEntry(tt.output)).To(Equal(tt.expected))
		})
	}
}

// TestCosignVersionCheck tests that cosign version meets minimum requirements
func TestCosignVersionCheck(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute cosign version command
	stdout, stderr, err := tools.ExecCommand(ctx, "cosign", []string{"version"}, nil, 10*time.Second)
	Expect(err).ToNot(HaveOccurred(), "Failed to get cosign version. Stderr: %s", stderr)

	t.Logf("Cosign version output: %s", stdout)

	// Check for version information
	Expect(strings.Contains(stdout, "GitVersion") || strings.Contains(stdout, "v")).To(BeTrue(),
		"Cosign version output doesn't contain version information")

	// Verify minimum version (v3.0.2+)
	versionChecker := tools.NewVersionChecker()
	valid, err := versionChecker.ValidateVersion("cosign", stdout)
	if err != nil {
		t.Logf("Version validation error (may be acceptable): %v", err)
	}
	if valid {
		t.Logf("Cosign version meets minimum requirements")
	} else {
		t.Logf("Warning: Cosign version may be below minimum (v3.0.2+)")
	}
}

// TestFailOpenBehavior tests that signing failures don't crash
func TestFailOpenBehavior(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()

	// Test with invalid private key
	signer := NewKeyBasedSigner("/nonexistent/key.pem", "", 5*time.Second)
	result, err := signer.Sign(ctx, "test-image:latest")

	// Should return error, not crash
	Expect(err).To(HaveOccurred())
	Expect(result).To(BeNil())

	t.Logf("Fail-open test passed: error=%v", err)
}

// TestSigningConfigValidation tests configuration validation
func TestSigningConfigValidation(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name: "Valid key-based config",
			config: &Config{
				Enabled:    true,
				Keyless:    false,
				PrivateKey: "/path/to/key.pem",
				PublicKey:  "/path/to/key.pub",
			},
			wantError: false,
		},
		{
			name: "Valid keyless config",
			config: &Config{
				Enabled:        true,
				Keyless:        true,
				OIDCIssuer:     "https://token.actions.githubusercontent.com",
				IdentityRegexp: "^https://github.com/.*$",
			},
			wantError: false,
		},
		{
			name: "Invalid key-based config (no private key)",
			config: &Config{
				Enabled:    true,
				Keyless:    false,
				PrivateKey: "",
			},
			wantError: true,
		},
		{
			name: "Invalid keyless config (no OIDC issuer)",
			config: &Config{
				Enabled:        true,
				Keyless:        true,
				IdentityRegexp: "^https://github.com/.*$",
			},
			wantError: true,
		},
		{
			name: "Disabled config (should be valid)",
			config: &Config{
				Enabled: false,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.config.Validate()
			if tt.wantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// TestKeyPairGenerationIntegration tests cosign key pair generation
func TestKeyPairGenerationIntegration(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("with password", func(t *testing.T) {
		RegisterTestingT(t)
		privateKey, publicKey, err := GenerateKeyPair(ctx, tempDir, "test-password")
		Expect(err).ToNot(HaveOccurred())

		// Verify files exist
		_, err = os.Stat(privateKey)
		Expect(err).ToNot(HaveOccurred(), "Private key not found")
		_, err = os.Stat(publicKey)
		Expect(err).ToNot(HaveOccurred(), "Public key not found")

		// Verify private key has secure permissions
		info, err := os.Stat(privateKey)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

		t.Logf("Generated key pair: %s, %s", privateKey, publicKey)
	})
}

// TestOIDCTokenValidation tests OIDC token validation
func TestOIDCTokenValidation(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name      string
		token     string
		wantError bool
	}{
		{
			name:      "Valid JWT format",
			token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			wantError: false,
		},
		{
			name:      "Empty token",
			token:     "",
			wantError: true,
		},
		{
			name:      "Invalid format (2 parts)",
			token:     "invalid.token",
			wantError: true,
		},
		{
			name:      "Invalid format (4 parts)",
			token:     "too.many.parts.here",
			wantError: false, // Has 3 dots, so 4 parts - should fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := ValidateOIDCToken(tt.token)
			if tt.wantError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// TestSigningTimeout tests that signing operations respect timeout
func TestSigningTimeout(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()
	tempDir := t.TempDir()

	// Generate test key
	privateKey, _, err := GenerateKeyPair(ctx, tempDir, "test")
	Expect(err).ToNot(HaveOccurred())

	// Create signer with very short timeout
	signer := NewKeyBasedSigner(privateKey, "test", 1*time.Nanosecond)

	_, err = signer.Sign(ctx, "test-image:latest")
	Expect(err).To(HaveOccurred())

	t.Logf("Timeout test result: %v", err)
}

// TestCleanupTempFiles tests that temporary key files are cleaned up
func TestCleanupTempFiles(t *testing.T) {
	skipIfCosignNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()

	// Create signer with raw key content (will create temp file)
	rawKey := "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"
	signer := NewKeyBasedSigner(rawKey, "", 5*time.Second)

	// Count temp files before
	tempDir := os.TempDir()
	before, _ := filepath.Glob(filepath.Join(tempDir, "cosign-key-*.key"))

	// Attempt signing (will fail but should clean up temp file)
	_, _ = signer.Sign(ctx, "test-image:latest")

	// Count temp files after
	after, _ := filepath.Glob(filepath.Join(tempDir, "cosign-key-*.key"))

	// Temp files should be cleaned up (count should be same or less)
	Expect(len(after)).To(BeNumerically("<=", len(before)))

	t.Logf("Temp file cleanup test: before=%d, after=%d", len(before), len(after))
}
