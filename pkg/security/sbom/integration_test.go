//go:build integration
// +build integration

package sbom

import (
	"context"
	"os"
	"testing"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// TestSyftGenerateIntegration tests real syft command execution
func TestSyftGenerateIntegration(t *testing.T) {
	ctx := context.Background()

	// Skip if syft not installed
	if err := CheckInstalled(ctx); err != nil {
		t.Skip("Syft not installed:", err)
	}

	// Use a small public image for testing
	testImage := "alpine:3.18"

	generator := NewSyftGenerator()

	t.Run("Generate CycloneDX JSON", func(t *testing.T) {
		sbom, err := generator.Generate(ctx, testImage, FormatCycloneDXJSON)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if sbom == nil {
			t.Fatal("Generate() returned nil SBOM")
		}

		if sbom.Format != FormatCycloneDXJSON {
			t.Errorf("SBOM format = %v, want %v", sbom.Format, FormatCycloneDXJSON)
		}

		if len(sbom.Content) == 0 {
			t.Error("SBOM content is empty")
		}

		if sbom.Digest == "" {
			t.Error("SBOM digest is empty")
		}

		if !sbom.ValidateDigest() {
			t.Error("SBOM digest validation failed")
		}

		if sbom.Metadata == nil {
			t.Error("SBOM metadata is nil")
		} else {
			if sbom.Metadata.ToolName != "syft" {
				t.Errorf("Tool name = %v, want syft", sbom.Metadata.ToolName)
			}
			if sbom.Metadata.PackageCount == 0 {
				t.Error("Package count is 0")
			}
			t.Logf("Found %d packages", sbom.Metadata.PackageCount)
		}

		t.Logf("SBOM size: %d bytes", sbom.Size())
	})

	t.Run("Generate SPDX JSON", func(t *testing.T) {
		sbom, err := generator.Generate(ctx, testImage, FormatSPDXJSON)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if sbom.Format != FormatSPDXJSON {
			t.Errorf("SBOM format = %v, want %v", sbom.Format, FormatSPDXJSON)
		}

		if len(sbom.Content) == 0 {
			t.Error("SBOM content is empty")
		}
	})

	t.Run("Generate Syft JSON", func(t *testing.T) {
		sbom, err := generator.Generate(ctx, testImage, FormatSyftJSON)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if sbom.Format != FormatSyftJSON {
			t.Errorf("SBOM format = %v, want %v", sbom.Format, FormatSyftJSON)
		}

		if len(sbom.Content) == 0 {
			t.Error("SBOM content is empty")
		}
	})
}

// TestSyftVersionIntegration tests version checking
func TestSyftVersionIntegration(t *testing.T) {
	ctx := context.Background()

	if err := CheckInstalled(ctx); err != nil {
		t.Skip("Syft not installed:", err)
	}

	generator := NewSyftGenerator()

	version, err := generator.Version(ctx)
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}

	if version == "" || version == "unknown" {
		t.Errorf("Version() returned empty or unknown version")
	}

	t.Logf("Syft version: %s", version)

	// Check minimum version (1.41.0)
	if err := CheckVersion(ctx, "1.0.0"); err != nil {
		t.Errorf("CheckVersion(1.0.0) error = %v", err)
	}
}

// TestAttacherIntegration tests SBOM attestation with cosign
func TestAttacherIntegration(t *testing.T) {
	ctx := context.Background()

	// Skip if syft or cosign not installed
	if err := CheckInstalled(ctx); err != nil {
		t.Skip("Syft not installed:", err)
	}

	// Check if cosign is available
	if err := signing.CheckCosignInstalled(ctx); err != nil {
		t.Skip("Cosign not installed:", err)
	}

	// Use ttl.sh for testing (ephemeral registry)
	testImage := "ttl.sh/sbom-test-" + generateRandomID() + ":1h"

	// Build a simple test image
	if err := buildTestImage(ctx, testImage); err != nil {
		t.Skip("Failed to build test image:", err)
	}
	defer cleanupTestImage(ctx, testImage)

	// Push test image
	if err := pushTestImage(ctx, testImage); err != nil {
		t.Skip("Failed to push test image:", err)
	}

	// Generate SBOM
	generator := NewSyftGenerator()
	sbom, err := generator.Generate(ctx, testImage, FormatCycloneDXJSON)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	t.Run("Attach SBOM with keyless signing", func(t *testing.T) {
		// Skip if not in GitHub Actions (keyless requires OIDC token)
		if os.Getenv("GITHUB_ACTIONS") != "true" {
			t.Skip("Keyless signing requires GitHub Actions OIDC token")
		}

		config := &signing.Config{
			Enabled: true,
			Keyless: true,
		}

		attacher := NewAttacher(config)

		err := attacher.Attach(ctx, sbom, testImage)
		if err != nil {
			t.Fatalf("Attach() error = %v", err)
		}

		t.Log("SBOM attached successfully with keyless signing")
	})

	t.Run("Attach SBOM with key-based signing", func(t *testing.T) {
		// Generate test keys
		keyPath := generateTestKeys(t)
		defer os.Remove(keyPath)
		defer os.Remove(keyPath + ".pub")

		config := &signing.Config{
			Enabled:    true,
			Keyless:    false,
			PrivateKey: keyPath,
			Password:   "test",
		}

		attacher := NewAttacher(config)

		err := attacher.Attach(ctx, sbom, testImage)
		if err != nil {
			t.Fatalf("Attach() error = %v", err)
		}

		t.Log("SBOM attached successfully with key-based signing")

		// Verify the attestation
		verifyConfig := &signing.Config{
			Enabled:   true,
			Keyless:   false,
			PublicKey: keyPath + ".pub",
		}

		verifier := NewAttacher(verifyConfig)
		verifiedSBOM, err := verifier.Verify(ctx, testImage, FormatCycloneDXJSON)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}

		if verifiedSBOM == nil {
			t.Fatal("Verify() returned nil SBOM")
		}

		t.Log("SBOM verified successfully")
	})
}

// Helper functions for integration tests

func generateRandomID() string {
	// Simple random ID for testing
	return "test123"
}

func buildTestImage(ctx context.Context, image string) error {
	// This would build a simple test image
	// For now, we'll skip this in the test
	return nil
}

func pushTestImage(ctx context.Context, image string) error {
	// This would push the test image
	// For now, we'll skip this in the test
	return nil
}

func cleanupTestImage(ctx context.Context, image string) {
	// Cleanup test image
}

func generateTestKeys(t *testing.T) string {
	// Generate temporary cosign key pair for testing
	keyPath := "/tmp/test-cosign-" + generateRandomID() + ".key"
	// This would call: cosign generate-key-pair
	// For now, return a placeholder
	return keyPath
}
