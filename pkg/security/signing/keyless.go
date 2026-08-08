// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/security/tools"
)

// execFn matches tools.ExecCommand; injectable for tests.
type execFn func(ctx context.Context, name string, args []string, env []string, timeout time.Duration) (string, string, error)

// MaxCosignAttempts bounds every Rekor-conflict retry loop: `cosign sign` here
// and `cosign attest` in the sbom and provenance packages.
const MaxCosignAttempts = 3

// IsRekorConflict reports a Rekor createLogEntryConflict (HTTP 409) — an
// identical entry already in the tlog, typically a cosign upload retry after
// a client-side timeout whose first attempt succeeded server-side.
func IsRekorConflict(output string) bool {
	return strings.Contains(output, "createLogEntryConflict") ||
		(strings.Contains(output, "409") && strings.Contains(output, "/api/v1/log/entries"))
}

// RetryOnRekorConflict runs attempt until it succeeds, fails for a reason other
// than a Rekor entry conflict, or exhausts MaxCosignAttempts.
//
// attempt must perform one complete cosign invocation and return the output to
// classify (stderr and stdout concatenated is fine) alongside its error. Each
// call has to be a fresh invocation: under keyless signing that mints a new
// ephemeral certificate, so the replayed Rekor body differs and the conflict
// clears. Deterministic keys reproduce the same signature and exhaust the loop,
// which is the correct outcome — a tlog entry does not prove the signature or
// attestation reached the registry, and cosign uploads to Rekor before it
// pushes to the registry.
//
// operation names the cosign subcommand for the retry warning, e.g. "attest".
func RetryOnRekorConflict(operation string, attempt func() (string, error)) error {
	var lastErr error
	for i := 1; i <= MaxCosignAttempts; i++ {
		output, err := attempt()
		if err == nil {
			return nil
		}
		lastErr = err
		if !IsRekorConflict(output) {
			return lastErr
		}
		fmt.Fprintf(os.Stderr, "Warning: Rekor transparency-log conflict on %s attempt %d/%d, retrying\n",
			operation, i, MaxCosignAttempts)
	}
	return lastErr
}

// runCosignSign retries the full `cosign sign` on Rekor entry conflicts.
// See RetryOnRekorConflict for why a retry — not a success — is the right
// response to a conflict.
func runCosignSign(ctx context.Context, exec execFn, args, env []string, timeout time.Duration) (string, error) {
	var signed string
	err := RetryOnRekorConflict("sign", func() (string, error) {
		stdout, stderr, err := exec(ctx, "cosign", args, env, timeout)
		if err == nil {
			signed = stdout
			return "", nil
		}
		return stderr + stdout, fmt.Errorf("cosign sign failed: %w\nStderr: %s\nStdout: %s", err, stderr, stdout)
	})
	if err != nil {
		return "", err
	}
	return signed, nil
}

// KeylessSigner implements keyless signing using OIDC tokens
type KeylessSigner struct {
	OIDCToken string
	Timeout   time.Duration

	// exec overrides command execution in tests; nil means tools.ExecCommand.
	exec execFn
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
	exec := s.exec
	if exec == nil {
		exec = tools.ExecCommand
	}
	stdout, err := runCosignSign(ctx, exec, args, env, s.Timeout)
	if err != nil {
		return nil, err
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

// ValidateOIDCToken performs basic format validation on the OIDC token.
// This checks JWT structure (3 dot-separated segments) only — it does NOT
// verify the signature, issuer, audience, or expiry. Full validation is
// performed by Fulcio when the token is exchanged for a signing certificate.
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
