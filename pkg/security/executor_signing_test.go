package security

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/security/signing"
)

// ExecuteSigning's pure pre-exec logic is reachable: OIDC token propagation,
// config validation, and signer construction. The terminal signer.Sign call
// shells out to cosign (absent here / nonexistent image), so it always fails
// and we assert the fail-open / fail-closed handling around it.

func TestExecuteSigningPropagatesOIDCTokenThenFailsOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cfg := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:        true,
			Required:       false, // fail-open
			Keyless:        true,
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
			IdentityRegexp: "^https://github.com/.*$",
			Timeout:        "1ms", // signer.Sign exec/network is bounded tightly
			// OIDCToken intentionally empty so the executor must inject it
			// from the execution context.
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())

	// Seed the execution context's OIDC token; ExecuteSigning copies it onto
	// the signing config so a keyless signer can be constructed.
	e.Context.OIDCToken = "fake-oidc-token"

	result, err := e.ExecuteSigning(ctx, "registry.example.com/demo@sha256:abc")
	// Sign fails (no cosign / unreachable image) but Required=false => fail-open.
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())

	// The token was propagated to the signing config.
	Expect(e.Config.Signing.OIDCToken).To(Equal("fake-oidc-token"))
	// A signing failure was recorded in the summary.
	Expect(e.Summary.SigningResult).ToNot(BeNil())
}

func TestExecuteSigningCreateSignerFailsFailOpen(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	// Valid keyless config (passes Validate) but no OIDC token anywhere, so
	// CreateSigner returns "OIDC token required". Required=false => fail-open.
	cfg := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:        true,
			Required:       false,
			Keyless:        true,
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
			IdentityRegexp: "^https://github.com/.*$",
			Timeout:        "5s",
		},
	}
	e, err := NewSecurityExecutorWithSummary(ctx, cfg, "img@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	e.Context.OIDCToken = "" // ensure no token to propagate

	result, err := e.ExecuteSigning(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())
	Expect(e.Summary.SigningResult).ToNot(BeNil())
}

func TestExecuteSigningCreateSignerFailsFailClosed(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cfg := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:        true,
			Required:       true, // fail-closed
			Keyless:        true,
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
			IdentityRegexp: "^https://github.com/.*$",
			Timeout:        "5s",
		},
	}
	e := newExecutorT(t, cfg)
	e.Context.OIDCToken = "" // no token => CreateSigner fails

	_, err := e.ExecuteSigning(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("creating signer"))
}

// Does not propagate the context token when the signing config already carries
// one (the executor only fills an empty OIDCToken).
func TestExecuteSigningDoesNotOverrideExistingToken(t *testing.T) {
	RegisterTestingT(t)
	ctx := context.Background()

	cfg := &SecurityConfig{
		Enabled: true,
		Signing: &signing.Config{
			Enabled:        true,
			Required:       false,
			Keyless:        true,
			OIDCIssuer:     "https://token.actions.githubusercontent.com",
			IdentityRegexp: "^https://github.com/.*$",
			OIDCToken:      "config-token",
			Timeout:        "1ms",
		},
	}
	e := newExecutorT(t, cfg)
	e.Context.OIDCToken = "context-token"

	_, err := e.ExecuteSigning(ctx, "registry.example.com/demo@sha256:abc")
	Expect(err).ToNot(HaveOccurred()) // fail-open on Sign exec failure

	// Pre-existing config token must be preserved.
	Expect(e.Config.Signing.OIDCToken).To(Equal("config-token"))
}
