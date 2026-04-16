package sbom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/simple-container-com/api/pkg/security/attestation"
	"github.com/simple-container-com/api/pkg/security/signing"
)

// Attacher handles SBOM attestation attachment to container images
type Attacher struct {
	// SigningConfig for attestation signing
	SigningConfig *signing.Config

	// Timeout for cosign commands
	Timeout time.Duration
}

// NewAttacher creates a new Attacher
func NewAttacher(signingConfig *signing.Config) *Attacher {
	return &Attacher{
		SigningConfig: signingConfig,
		Timeout:       2 * time.Minute,
	}
}

// Attach attaches an SBOM as a signed attestation to an image
func (a *Attacher) Attach(ctx context.Context, sbom *SBOM, image string) error {
	// Create temporary file for SBOM
	tmpFile, err := a.createTempSBOMFile(sbom)
	if err != nil {
		return fmt.Errorf("failed to create temp SBOM file: %w", err)
	}
	defer os.Remove(tmpFile)

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	// Build cosign attest command
	args := []string{
		"attest",
		"--predicate", tmpFile,
		"--type", sbom.Format.AttestationType(),
	}

	// Add signing configuration
	args = append(args, a.buildSigningArgs()...)

	// Add image
	args = append(args, image)

	cmd := exec.CommandContext(timeoutCtx, "cosign", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set environment variables for signing
	cmd.Env = append(os.Environ(), a.buildSigningEnv()...)

	// Execute cosign attest
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cosign attest failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// Verify verifies an SBOM attestation
func (a *Attacher) Verify(ctx context.Context, image string, format Format) (*SBOM, error) {
	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	// Build cosign verify-attestation command
	args := []string{
		"verify-attestation",
		"--type", format.AttestationType(),
	}

	// Add verification configuration
	args = append(args, a.buildVerificationArgs()...)

	// Add image
	args = append(args, image)

	cmd := exec.CommandContext(timeoutCtx, "cosign", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set environment variables
	cmd.Env = append(os.Environ(), a.buildSigningEnv()...)

	// Execute cosign verify-attestation
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cosign verify-attestation failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse attestation output to extract SBOM
	sbom, err := a.parseAttestationOutput(stdout.Bytes(), format, image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse attestation output: %w", err)
	}

	return sbom, nil
}

// createTempSBOMFile creates a temporary file with SBOM content.
// Uses os.CreateTemp for unpredictable filenames and secure creation.
func (a *Attacher) createTempSBOMFile(sbomData *SBOM) (string, error) {
	f, err := os.CreateTemp("", "sbom-*.json")
	if err != nil {
		return "", fmt.Errorf("creating temp SBOM file: %w", err)
	}
	path := f.Name()

	if _, err := f.Write(sbomData.Content); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("writing temp SBOM file: %w", err)
	}
	f.Close()
	return path, nil
}

// buildSigningArgs builds cosign signing arguments
func (a *Attacher) buildSigningArgs() []string {
	var args []string

	if a.SigningConfig == nil {
		return args
	}

	// Keyless signing
	if a.SigningConfig.Keyless {
		args = append(args, "--yes") // Auto-confirm for keyless
	} else if a.SigningConfig.PrivateKey != "" {
		// Key-based signing
		args = append(args, "--key", a.SigningConfig.PrivateKey)
	}

	return args
}

// buildVerificationArgs builds cosign verification arguments
func (a *Attacher) buildVerificationArgs() []string {
	var args []string

	if a.SigningConfig == nil {
		return args
	}

	// Keyless verification (use certificate identity)
	if a.SigningConfig.Keyless {
		if a.SigningConfig.IdentityRegexp != "" {
			args = append(args, "--certificate-identity-regexp", a.SigningConfig.IdentityRegexp)
		}
		if a.SigningConfig.OIDCIssuer != "" {
			args = append(args, "--certificate-oidc-issuer", a.SigningConfig.OIDCIssuer)
		}
	} else if a.SigningConfig.PublicKey != "" {
		// Key-based verification
		args = append(args, "--key", a.SigningConfig.PublicKey)
	}

	return args
}

// buildSigningEnv builds environment variables for cosign attestation commands.
func (a *Attacher) buildSigningEnv() []string {
	if a.SigningConfig == nil {
		return nil
	}

	var env []string
	if a.SigningConfig.Keyless && a.SigningConfig.OIDCToken != "" {
		env = append(env, "SIGSTORE_ID_TOKEN="+a.SigningConfig.OIDCToken)
	}
	if !a.SigningConfig.Keyless && a.SigningConfig.PrivateKey != "" {
		env = append(env, "COSIGN_PASSWORD="+a.SigningConfig.Password)
	}
	return env
}

// parseAttestationOutput parses the cosign verify-attestation output
func (a *Attacher) parseAttestationOutput(output []byte, format Format, image string) (*SBOM, error) {
	// The payload contains the verified in-toto statement. Extract the predicate
	// as the verified SBOM document.
	var statement struct {
		Predicate json.RawMessage `json:"predicate"`
	}

	payloadBytes, err := attestation.DecodeFirstPayload(output)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payloadBytes, &statement); err != nil {
		return nil, fmt.Errorf("failed to parse attestation payload: %w", err)
	}
	if len(statement.Predicate) == 0 || string(statement.Predicate) == "null" {
		return nil, fmt.Errorf("attestation has no predicate — SBOM content missing")
	}

	// Extract image digest
	imageDigest := a.extractImageDigest(image)

	// Create SBOM from predicate
	sbom := NewSBOM(format, statement.Predicate, imageDigest, &Metadata{
		ToolName:    "syft",
		ToolVersion: "unknown",
	})

	return sbom, nil
}

// extractImageDigest extracts the image digest from image reference
func (a *Attacher) extractImageDigest(image string) string {
	// Extract digest if present in image reference
	digestRegex := regexp.MustCompile(`sha256:[a-f0-9]{64}`)
	if matches := digestRegex.FindString(image); matches != "" {
		return matches
	}
	return image
}
