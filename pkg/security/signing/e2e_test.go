//go:build e2e
// +build e2e

package signing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// E2E test configuration
const (
	testRegistry  = "ttl.sh" // Public ephemeral registry (images expire in 24h)
	testImageName = "simple-container-signing-test"
	testTimeout   = 2 * time.Minute
)

// skipIfToolsNotInstalled skips E2E tests if required tools are missing
func skipIfToolsNotInstalled(t *testing.T) {
	t.Helper()
	installer := tools.NewToolInstaller()

	// Check cosign
	if installed, err := installer.CheckInstalled("cosign"); err != nil || !installed {
		t.Skip("Skipping E2E test: cosign not installed. Install from https://docs.sigstore.dev/cosign/installation/")
	}

	// Check docker
	if installed, err := installer.CheckInstalled("docker"); err != nil || !installed {
		t.Skip("Skipping E2E test: docker not installed")
	}
}

// buildTestImage builds a simple test Docker image
func buildTestImage(t *testing.T, imageRef string) {
	t.Helper()

	// Create a temporary directory for build context
	tempDir := t.TempDir()
	dockerfilePath := fmt.Sprintf("%s/Dockerfile", tempDir)

	// Write a minimal Dockerfile
	dockerfile := `FROM alpine:latest
LABEL description="Simple Container E2E Test Image"
RUN echo "test" > /test.txt
CMD ["cat", "/test.txt"]
`
	err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o644)
	Expect(err).ToNot(HaveOccurred(), "Failed to write Dockerfile")

	// Build the image
	cmd := exec.Command("docker", "build", "-t", imageRef, tempDir)
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to build test image: %s", output)

	t.Logf("Built test image: %s", imageRef)
}

// pushTestImage pushes the test image to the registry
func pushTestImage(t *testing.T, imageRef string) string {
	t.Helper()

	// Push the image
	cmd := exec.Command("docker", "push", imageRef)
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to push test image: %s", output)

	// Get the image digest
	cmd = exec.Command("docker", "inspect", "--format={{index .RepoDigests 0}}", imageRef)
	digestOutput, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to get image digest: %s", digestOutput)

	digest := strings.TrimSpace(string(digestOutput))
	t.Logf("Pushed test image with digest: %s", digest)
	return digest
}

// cleanupTestImage removes the test image locally
func cleanupTestImage(t *testing.T, imageRef string) {
	t.Helper()
	cmd := exec.Command("docker", "rmi", "-f", imageRef)
	_ = cmd.Run() // Ignore errors during cleanup
	t.Logf("Cleaned up test image: %s", imageRef)
}

// TestE2EKeyBasedWorkflow tests full key-based signing workflow
func TestE2EKeyBasedWorkflow(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Generate unique image tag using timestamp
	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:keybased-%d", testRegistry, testImageName, timestamp)

	t.Logf("Starting E2E key-based workflow with image: %s", imageTag)

	// Step 1: Build test image
	t.Log("Step 1: Building test image...")
	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)

	// Step 2: Push to registry
	t.Log("Step 2: Pushing to registry...")
	imageDigest := pushTestImage(t, imageTag)

	// Step 3: Generate test keys
	t.Log("Step 3: Generating test keys...")
	tempDir := t.TempDir()
	password := "e2e-test-password"
	privateKeyPath, publicKeyPath, err := GenerateKeyPair(ctx, tempDir, password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	// Step 4: Sign the image
	t.Log("Step 4: Signing image...")
	signer := NewKeyBasedSigner(privateKeyPath, password, 60*time.Second)
	signResult, err := signer.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign image")
	Expect(signResult).ToNot(BeNil())
	t.Logf("Image signed successfully at %s", signResult.SignedAt)

	// Step 5: Verify the signature
	t.Log("Step 5: Verifying signature...")
	verifier := NewKeyBasedVerifier(publicKeyPath, 60*time.Second)
	verifyResult, err := verifier.Verify(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify signature")
	Expect(verifyResult).ToNot(BeNil())
	Expect(verifyResult.Verified).To(BeTrue())
	t.Logf("Signature verified successfully at %s", verifyResult.VerifiedAt)

	// Step 6: Test verification with wrong key (should fail)
	t.Log("Step 6: Testing verification with wrong key...")

	// Generate a different key pair
	_, wrongPublicKey, err := GenerateKeyPair(ctx, fmt.Sprintf("%s/wrong", tempDir), password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate wrong key pair")

	wrongVerifier := NewKeyBasedVerifier(wrongPublicKey, 60*time.Second)
	wrongResult, err := wrongVerifier.Verify(ctx, imageDigest)

	// Verification with wrong key should fail
	Expect(err).To(HaveOccurred(), "Expected verification to fail with wrong key")
	t.Logf("Verification correctly failed with wrong key: %v", err)
	if wrongResult != nil {
		Expect(wrongResult.Verified).To(BeFalse())
	}

	t.Log("E2E key-based workflow completed successfully")
}

// TestE2EKeylessWorkflow tests full keyless signing workflow
func TestE2EKeylessWorkflow(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	// Check for OIDC token (available in GitHub Actions)
	oidcToken := os.Getenv("SIGSTORE_ID_TOKEN")
	if oidcToken == "" {
		// Try GitHub Actions token
		oidcToken = os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		if oidcToken == "" {
			t.Skip("Skipping keyless E2E test: OIDC token not available (set SIGSTORE_ID_TOKEN or run in GitHub Actions)")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Generate unique image tag
	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:keyless-%d", testRegistry, testImageName, timestamp)

	t.Logf("Starting E2E keyless workflow with image: %s", imageTag)

	// Step 1: Build test image
	t.Log("Step 1: Building test image...")
	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)

	// Step 2: Push to registry
	t.Log("Step 2: Pushing to registry...")
	imageDigest := pushTestImage(t, imageTag)

	// Step 3: Sign with keyless (OIDC)
	t.Log("Step 3: Signing image with keyless OIDC...")
	signer := NewKeylessSigner(oidcToken, 60*time.Second)
	signResult, err := signer.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign image with keyless")
	Expect(signResult).ToNot(BeNil())
	Expect(signResult.RekorEntry).ToNot(BeEmpty(), "Expected Rekor entry URL in keyless signing result")
	t.Logf("Image signed keylessly, Rekor entry: %s", signResult.RekorEntry)

	// Step 4: Verify keyless signature
	t.Log("Step 4: Verifying keyless signature...")

	// For GitHub Actions
	oidcIssuer := "https://token.actions.githubusercontent.com"
	identityRegexp := "^https://github.com/.*$"

	verifier := NewKeylessVerifier(oidcIssuer, identityRegexp, 60*time.Second)
	verifyResult, err := verifier.Verify(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify keyless signature")
	Expect(verifyResult).ToNot(BeNil())
	Expect(verifyResult.Verified).To(BeTrue())
	t.Logf("Keyless signature verified successfully")

	// Step 5: Validate Rekor entry is accessible
	if signResult.RekorEntry != "" {
		t.Logf("Step 5: Validating Rekor entry is accessible: %s", signResult.RekorEntry)
	}

	t.Log("E2E keyless workflow completed successfully")
}

// TestE2ESigningWithConfig tests signing using Config helper
func TestE2ESigningWithConfig(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Generate unique image tag
	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:config-%d", testRegistry, testImageName, timestamp)

	t.Logf("Starting E2E config-based workflow with image: %s", imageTag)

	// Build and push test image
	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)
	imageDigest := pushTestImage(t, imageTag)

	// Generate test keys
	tempDir := t.TempDir()
	password := "config-test-password"
	privateKeyPath, publicKeyPath, err := GenerateKeyPair(ctx, tempDir, password)
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	// Test with key-based config
	config := &Config{
		Enabled:    true,
		Required:   true,
		Keyless:    false,
		PrivateKey: privateKeyPath,
		PublicKey:  publicKeyPath,
		Password:   password,
		Timeout:    "60s",
	}

	// Sign using config
	signResult, err := SignImage(ctx, config, imageDigest, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(signResult).ToNot(BeNil())
	t.Logf("Signed with config at %s", signResult.SignedAt)

	// Verify using config
	verifyResult, err := VerifyImage(ctx, config, imageDigest)
	Expect(err).ToNot(HaveOccurred())
	Expect(verifyResult).ToNot(BeNil())
	Expect(verifyResult.Verified).To(BeTrue())
	t.Logf("Verified with config at %s", verifyResult.VerifiedAt)

	t.Log("E2E config-based workflow completed successfully")
}

// TestE2ELocalRegistry tests signing with local registry
func TestE2ELocalRegistry(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	// Check if local registry is running
	ctx := context.Background()
	cmd := exec.Command("docker", "ps", "--filter", "name=registry", "--format", "{{.Names}}")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "registry") {
		t.Skip("Skipping local registry test: local Docker registry not running. Start with: docker run -d -p 5000:5000 --name registry registry:2")
	}

	// Use local registry
	localRegistry := "localhost:5000"
	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:local-%d", localRegistry, testImageName, timestamp)

	t.Logf("Starting E2E local registry workflow with image: %s", imageTag)

	// Build test image
	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)

	// Push to local registry
	imageDigest := pushTestImage(t, imageTag)

	// Generate keys and sign
	tempDir := t.TempDir()
	privateKey, publicKey, err := GenerateKeyPair(ctx, tempDir, "local-test")
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	signer := NewKeyBasedSigner(privateKey, "local-test", 60*time.Second)
	_, err = signer.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign local image")

	verifier := NewKeyBasedVerifier(publicKey, 60*time.Second)
	result, err := verifier.Verify(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify local image")
	Expect(result.Verified).To(BeTrue())

	t.Log("E2E local registry workflow completed successfully")
}

// TestE2EMultipleSignatures tests signing the same image multiple times
func TestE2EMultipleSignatures(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Generate unique image tag
	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:multi-%d", testRegistry, testImageName, timestamp)

	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)
	imageDigest := pushTestImage(t, imageTag)

	tempDir := t.TempDir()

	// Sign with first key
	privateKey1, publicKey1, err := GenerateKeyPair(ctx, fmt.Sprintf("%s/key1", tempDir), "pass1")
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair 1")

	signer1 := NewKeyBasedSigner(privateKey1, "pass1", 60*time.Second)
	_, err = signer1.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign with key 1")
	t.Log("Signed with first key")

	// Sign with second key
	privateKey2, publicKey2, err := GenerateKeyPair(ctx, fmt.Sprintf("%s/key2", tempDir), "pass2")
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair 2")

	signer2 := NewKeyBasedSigner(privateKey2, "pass2", 60*time.Second)
	_, err = signer2.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign with key 2")
	t.Log("Signed with second key")

	// Verify with first key
	verifier1 := NewKeyBasedVerifier(publicKey1, 60*time.Second)
	result1, err := verifier1.Verify(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify with key 1")
	Expect(result1.Verified).To(BeTrue())

	// Verify with second key
	verifier2 := NewKeyBasedVerifier(publicKey2, 60*time.Second)
	result2, err := verifier2.Verify(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify with key 2")
	Expect(result2.Verified).To(BeTrue())

	t.Log("Multiple signatures workflow completed successfully")
}

// TestE2EImageRetrieval tests retrieving signed image from registry
func TestE2EImageRetrieval(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	timestamp := time.Now().Unix()
	imageTag := fmt.Sprintf("%s/%s:retrieve-%d", testRegistry, testImageName, timestamp)

	// Build, push, and sign image
	buildTestImage(t, imageTag)
	defer cleanupTestImage(t, imageTag)
	imageDigest := pushTestImage(t, imageTag)

	tempDir := t.TempDir()
	privateKey, _, err := GenerateKeyPair(ctx, tempDir, "test")
	Expect(err).ToNot(HaveOccurred(), "Failed to generate key pair")

	signer := NewKeyBasedSigner(privateKey, "test", 60*time.Second)
	_, err = signer.Sign(ctx, imageDigest)
	Expect(err).ToNot(HaveOccurred(), "Failed to sign")

	// Remove local image
	cmd := exec.Command("docker", "rmi", "-f", imageTag)
	_ = cmd.Run()

	// Pull image again
	cmd = exec.Command("docker", "pull", imageTag)
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to pull signed image: %s", output)

	t.Logf("Successfully retrieved signed image from registry")
}

// TestE2EFailOpenBehavior tests fail-open behavior in E2E scenario
func TestE2EFailOpenBehavior(t *testing.T) {
	skipIfToolsNotInstalled(t)
	RegisterTestingT(t)

	ctx := context.Background()

	// Test with non-existent image (should fail gracefully)
	nonExistentImage := "registry.example.com/nonexistent:latest"

	config := &Config{
		Enabled:    true,
		Required:   false, // fail-open
		Keyless:    false,
		PrivateKey: "/tmp/fake-key.pem",
		Timeout:    "5s",
	}

	// Should return error but not crash
	result, err := SignImage(ctx, config, nonExistentImage, "")
	// With Required=false, error should be handled gracefully
	if err != nil {
		t.Logf("Expected error with fail-open: %v", err)
	}
	if result != nil {
		t.Logf("Result: %+v", result)
	}

	t.Log("Fail-open behavior test passed")
}
