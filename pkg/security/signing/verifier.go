package signing

import (
	"context"
	"fmt"
	"time"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// VerifyResult contains the result of a signature verification
type VerifyResult struct {
	Verified        bool
	ImageDigest     string
	CertificateInfo *CertificateInfo
	VerifiedAt      string
}

// CertificateInfo contains information about the signing certificate
type CertificateInfo struct {
	Issuer   string
	Subject  string
	Identity string
}

// Verifier handles signature verification for container images
type Verifier struct {
	// For keyless verification
	OIDCIssuer     string
	IdentityRegexp string

	// For key-based verification
	PublicKey string // Path to public key file

	Timeout time.Duration
}

// NewKeylessVerifier creates a verifier for keyless signatures
func NewKeylessVerifier(oidcIssuer, identityRegexp string, timeout time.Duration) *Verifier {
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	return &Verifier{
		OIDCIssuer:     oidcIssuer,
		IdentityRegexp: identityRegexp,
		Timeout:        timeout,
	}
}

// NewKeyBasedVerifier creates a verifier for key-based signatures
func NewKeyBasedVerifier(publicKey string, timeout time.Duration) *Verifier {
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	return &Verifier{
		PublicKey: publicKey,
		Timeout:   timeout,
	}
}

// Verify verifies the signature of a container image
func (v *Verifier) Verify(ctx context.Context, imageRef string) (*VerifyResult, error) {
	var args []string
	var env []string

	if v.PublicKey != "" {
		// Key-based verification
		args = []string{"verify", "--key", v.PublicKey, imageRef}
	} else if v.OIDCIssuer != "" && v.IdentityRegexp != "" {
		// Keyless verification
		args = []string{
			"verify",
			"--certificate-oidc-issuer", v.OIDCIssuer,
			"--certificate-identity-regexp", v.IdentityRegexp,
			imageRef,
		}
		env = []string{"COSIGN_EXPERIMENTAL=1"}
	} else {
		return nil, fmt.Errorf("verifier requires either public key or OIDC issuer + identity regexp")
	}

	stdout, stderr, err := tools.ExecCommand(ctx, "cosign", args, env, v.Timeout)

	result := &VerifyResult{
		Verified:   err == nil,
		VerifiedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err != nil {
		return result, fmt.Errorf("cosign verify failed: %w\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// Parse certificate information from output if available
	result.CertificateInfo = parseCertificateInfo(stdout)

	return result, nil
}

// parseCertificateInfo extracts certificate information from cosign verify output
func parseCertificateInfo(output string) *CertificateInfo {
	// Cosign verify output includes certificate details in JSON format
	// For now, return empty certificate info - can be enhanced later
	return &CertificateInfo{}
}

// VerifyWithPolicy verifies a signature and applies additional policy checks
func (v *Verifier) VerifyWithPolicy(ctx context.Context, imageRef string, policy PolicyChecker) (*VerifyResult, error) {
	result, err := v.Verify(ctx, imageRef)
	if err != nil {
		return result, err
	}

	if policy != nil {
		if err := policy.Check(result); err != nil {
			result.Verified = false
			return result, fmt.Errorf("policy check failed: %w", err)
		}
	}

	return result, nil
}

// PolicyChecker is an interface for custom verification policies
type PolicyChecker interface {
	Check(result *VerifyResult) error
}
