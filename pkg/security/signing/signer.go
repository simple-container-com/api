package signing

import (
	"context"
)

// SignResult contains the result of a signing operation
type SignResult struct {
	ImageDigest string
	Signature   string
	Bundle      string
	RekorEntry  string // URL to Rekor transparency log entry
	SignedAt    string
}

// Signer is the interface for signing container images
type Signer interface {
	// Sign signs a container image and returns the result
	Sign(ctx context.Context, imageRef string) (*SignResult, error)
}

// SignerConfig contains common configuration for signers
type SignerConfig struct {
	// Required indicates whether signing is required (fail-closed) or optional (fail-open)
	Required bool
	// Timeout for signing operation
	Timeout string
}
