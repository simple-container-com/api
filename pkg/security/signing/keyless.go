package signing

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// KeylessSigner implements keyless signing using OIDC tokens
type KeylessSigner struct {
	OIDCToken string
	Timeout   time.Duration
}

// NewKeylessSigner creates a new keyless signer
func NewKeylessSigner(oidcToken string, timeout time.Duration) *KeylessSigner {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &KeylessSigner{
		OIDCToken: oidcToken,
		Timeout:   timeout,
	}
}

// Sign signs a container image using keyless OIDC signing
func (s *KeylessSigner) Sign(ctx context.Context, imageRef string) (*SignResult, error) {
	if s.OIDCToken == "" {
		return nil, fmt.Errorf("OIDC token is required for keyless signing")
	}

	// Prepare environment variables
	env := []string{
		"COSIGN_EXPERIMENTAL=1",
		"SIGSTORE_ID_TOKEN=" + s.OIDCToken,
	}

	// Execute cosign sign command
	args := []string{"sign", "--yes", imageRef}
	stdout, stderr, err := tools.ExecCommand(ctx, "cosign", args, env, s.Timeout)
	if err != nil {
		return nil, fmt.Errorf("cosign sign failed: %w\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// Parse output for Rekor entry URL
	rekorEntry := parseRekorEntry(stdout)

	result := &SignResult{
		RekorEntry: rekorEntry,
		SignedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	return result, nil
}

// parseRekorEntry extracts the Rekor entry URL from cosign output
func parseRekorEntry(output string) string {
	// Look for Rekor entry patterns in output
	// Example: "tlog entry created with index: 123456789"
	// or "https://rekor.sigstore.dev/api/v1/log/entries/..."

	// Check for direct URL
	urlRegex := regexp.MustCompile(`https://[^\s]*rekor[^\s]*`)
	if matches := urlRegex.FindString(output); matches != "" {
		return matches
	}

	// Check for index reference
	indexRegex := regexp.MustCompile(`tlog entry created with index:\s*(\d+)`)
	if matches := indexRegex.FindStringSubmatch(output); len(matches) > 1 {
		return fmt.Sprintf("https://rekor.sigstore.dev/api/v1/log/entries?logIndex=%s", matches[1])
	}

	return ""
}

// GetRekorEntryFromOutput parses cosign output to extract Rekor entry information
func GetRekorEntryFromOutput(output string) string {
	return parseRekorEntry(output)
}

// ValidateOIDCToken performs basic validation on the OIDC token
func ValidateOIDCToken(token string) error {
	if token == "" {
		return fmt.Errorf("OIDC token is empty")
	}

	// JWT tokens have 3 parts separated by dots
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid OIDC token format: expected 3 parts, got %d", len(parts))
	}

	return nil
}
