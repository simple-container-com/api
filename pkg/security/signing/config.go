// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package signing

import (
	"context"
	"fmt"
	"time"
)

// Config contains configuration for image signing operations
type Config struct {
	Enabled    bool
	Required   bool
	Keyless    bool
	PrivateKey string
	PublicKey  string
	// Password is the cosign private key passphrase.
	// json:"-" prevents accidental serialization if this struct is ever marshaled
	// as part of a larger config object (e.g., debug logging, cache key hashing).
	// It is always populated programmatically from CLI flags or env vars, never loaded from JSON.
	Password string `json:"-" yaml:"-"`
	Timeout  string

	// OIDCToken is the OIDC identity token for keyless signing/attestation.
	// Set at runtime from CI environment (ACTIONS_ID_TOKEN_REQUEST_*).
	// Used by SBOM attacher and provenance attacher for cosign attestations.
	OIDCToken string `json:"-" yaml:"-"`

	// Verification settings
	OIDCIssuer     string
	IdentityRegexp string
}

// CreateSigner creates a signer based on the configuration.
// The oidcToken parameter takes precedence; falls back to c.OIDCToken if empty.
func (c *Config) CreateSigner(oidcToken string) (Signer, error) {
	timeout, err := parseDuration(c.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout: %w", err)
	}

	if c.Keyless {
		token := oidcToken
		if token == "" {
			token = c.OIDCToken
		}
		if token == "" {
			return nil, fmt.Errorf("OIDC token required for keyless signing")
		}
		signer := NewKeylessSigner(token, timeout)
		// Needed to confirm a Rekor conflict against the image; see
		// KeylessSigner.IdentityRegexp.
		signer.IdentityRegexp = c.IdentityRegexp
		signer.OIDCIssuer = c.OIDCIssuer
		return signer, nil
	}

	if c.PrivateKey == "" {
		return nil, fmt.Errorf("private key required for key-based signing")
	}

	return NewKeyBasedSigner(c.PrivateKey, c.Password, timeout), nil
}

// CreateVerifier creates a verifier based on the configuration
func (c *Config) CreateVerifier() (*Verifier, error) {
	timeout, err := parseDuration(c.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout: %w", err)
	}

	if c.Keyless {
		if c.OIDCIssuer == "" || c.IdentityRegexp == "" {
			return nil, fmt.Errorf("OIDC issuer and identity regexp required for keyless verification")
		}
		return NewKeylessVerifier(c.OIDCIssuer, c.IdentityRegexp, timeout), nil
	}

	if c.PublicKey == "" {
		return nil, fmt.Errorf("public key required for key-based verification")
	}

	return NewKeyBasedVerifier(c.PublicKey, timeout), nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Keyless {
		// Keyless signing validation
		if c.OIDCIssuer == "" {
			return fmt.Errorf("oidc_issuer required for keyless signing")
		}
		if c.IdentityRegexp == "" {
			return fmt.Errorf("identity_regexp required for keyless signing")
		}
	} else {
		// Key-based signing validation
		if c.PrivateKey == "" {
			return fmt.Errorf("private_key required for key-based signing")
		}
	}

	return nil
}

// parseDuration parses a duration string with default fallback
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 5 * time.Minute, nil
	}
	return time.ParseDuration(s)
}

// SignImage is a convenience function to sign an image with the given configuration
func SignImage(ctx context.Context, config *Config, imageRef string, oidcToken string) (*SignResult, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("signing is not enabled")
	}

	signer, err := config.CreateSigner(oidcToken)
	if err != nil {
		return nil, fmt.Errorf("creating signer: %w", err)
	}

	return signer.Sign(ctx, imageRef)
}

// VerifyImage is a convenience function to verify an image with the given configuration
func VerifyImage(ctx context.Context, config *Config, imageRef string) (*VerifyResult, error) {
	verifier, err := config.CreateVerifier()
	if err != nil {
		return nil, fmt.Errorf("creating verifier: %w", err)
	}

	return verifier.Verify(ctx, imageRef)
}

// verificationIdentityArgs returns the cosign flags that bind a verification to
// the identity this config signs with. Empty when the config cannot express
// one, which is the signal to skip conflict confirmation entirely: a probe that
// only asks whether *some* artifact is attached would accept one produced under
// a rotated key or by an unrelated workflow.
func (c *Config) verificationIdentityArgs() []string {
	if c == nil {
		return nil
	}

	if c.Keyless {
		if c.IdentityRegexp == "" || c.OIDCIssuer == "" {
			return nil
		}
		return []string{
			"--certificate-identity-regexp", c.IdentityRegexp,
			"--certificate-oidc-issuer", c.OIDCIssuer,
		}
	}

	// cosign resolves --key against a public key, a KMS URI or a private key,
	// deriving the public half where needed, so a config carrying only the
	// signing key can still confirm against the identity it just signed with.
	key := c.PublicKey
	if key == "" {
		key = c.PrivateKey
	}
	if key == "" {
		return nil
	}
	return []string{"--key", key}
}

// AttestationConfirmProbe builds the read-only check that decides whether a
// Rekor conflict on `cosign attest --type predicateType` is an idempotent
// no-op. Nil when the config names no verification identity.
func (c *Config) AttestationConfirmProbe(predicateType string) *ConfirmProbe {
	identity := c.verificationIdentityArgs()
	if len(identity) == 0 || predicateType == "" {
		return nil
	}
	args := append([]string{"verify-attestation", "--type", predicateType}, identity...)
	return &ConfirmProbe{Args: args, What: predicateType + " attestation"}
}

// SignatureConfirmProbe is AttestationConfirmProbe's twin for `cosign sign`.
func (c *Config) SignatureConfirmProbe() *ConfirmProbe {
	identity := c.verificationIdentityArgs()
	if len(identity) == 0 {
		return nil
	}
	return &ConfirmProbe{Args: append([]string{"verify"}, identity...), What: "signature"}
}
