// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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

	// Confirmed reports that the signature was not produced by this run: the
	// transparency log already held an identical entry and a verification probe
	// confirmed the signature is on the image. Nothing fresh was emitted, so
	// RekorEntry is empty even though the operation succeeded.
	Confirmed bool
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
