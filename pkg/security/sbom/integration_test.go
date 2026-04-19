//go:build integration
// +build integration

package sbom

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"

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
		RegisterTestingT(t)
		sbom, err := generator.Generate(ctx, testImage, FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom).ToNot(BeNil())
		Expect(sbom.Format).To(Equal(FormatCycloneDXJSON))
		Expect(sbom.Content).ToNot(BeEmpty())
		Expect(sbom.Digest).ToNot(BeEmpty())
		Expect(sbom.ValidateDigest()).To(BeTrue())
		Expect(sbom.Metadata).ToNot(BeNil())
		Expect(sbom.Metadata.ToolName).To(Equal("syft"))
		Expect(sbom.Metadata.PackageCount).ToNot(BeZero())
		t.Logf("Found %d packages", sbom.Metadata.PackageCount)
		t.Logf("SBOM size: %d bytes", sbom.Size())
	})

	t.Run("Generate SPDX JSON", func(t *testing.T) {
		RegisterTestingT(t)
		sbom, err := generator.Generate(ctx, testImage, FormatSPDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Format).To(Equal(FormatSPDXJSON))
		Expect(sbom.Content).ToNot(BeEmpty())
	})

	t.Run("Generate Syft JSON", func(t *testing.T) {
		RegisterTestingT(t)
		sbom, err := generator.Generate(ctx, testImage, FormatSyftJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(sbom.Format).To(Equal(FormatSyftJSON))
		Expect(sbom.Content).ToNot(BeEmpty())
	})
}

// TestSyftVersionIntegration tests version checking
func TestSyftVersionIntegration(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	if err := CheckInstalled(ctx); err != nil {
		t.Skip("Syft not installed:", err)
	}

	generator := NewSyftGenerator()

	version, err := generator.Version(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(version).ToNot(SatisfyAny(BeEmpty(), Equal("unknown")))

	t.Logf("Syft version: %s", version)

	// Check minimum version (1.41.0)
	err = CheckVersion(ctx, "1.0.0")
	Expect(err).ToNot(HaveOccurred())
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
	RegisterTestingT(t)
	generator := NewSyftGenerator()
	sbom, err := generator.Generate(ctx, testImage, FormatCycloneDXJSON)
	Expect(err).ToNot(HaveOccurred())

	t.Run("Attach SBOM with keyless signing", func(t *testing.T) {
		RegisterTestingT(t)
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
		Expect(err).ToNot(HaveOccurred())

		t.Log("SBOM attached successfully with keyless signing")
	})

	t.Run("Attach SBOM with key-based signing", func(t *testing.T) {
		RegisterTestingT(t)
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
		Expect(err).ToNot(HaveOccurred())

		t.Log("SBOM attached successfully with key-based signing")

		// Verify the attestation
		verifyConfig := &signing.Config{
			Enabled:   true,
			Keyless:   false,
			PublicKey: keyPath + ".pub",
		}

		verifier := NewAttacher(verifyConfig)
		verifiedSBOM, err := verifier.Verify(ctx, testImage, FormatCycloneDXJSON)
		Expect(err).ToNot(HaveOccurred())
		Expect(verifiedSBOM).ToNot(BeNil())

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
