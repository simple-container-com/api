// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package ciphers

import (
	"crypto/rand"

	"github.com/pkg/errors"
	"golang.org/x/crypto/chacha20poly1305"
)

// DEKSize is the length of a data-encryption key for the envelope AEAD.
const DEKSize = chacha20poly1305.KeySize

// GenerateDEK returns a fresh random 32-byte data-encryption key.
func GenerateDEK() ([]byte, error) {
	k := make([]byte, DEKSize)
	if _, err := rand.Read(k); err != nil {
		return nil, errors.Wrap(err, "failed to generate data key")
	}
	return k, nil
}

// SealAEAD encrypts plaintext under a 32-byte key with ChaCha20-Poly1305, binding
// aad, and returns nonce(12) || ciphertext+tag. This is the envelope value blob:
// the value is encrypted ONCE with a random DEK and the DEK is wrapped per
// recipient, so all recipients see the same value (no per-recipient divergence)
// and a single whole-value MAC prevents chunk splicing.
func SealAEAD(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AEAD")
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "failed to generate nonce")
	}
	ct := aead.Seal(nil, nonce, plaintext, aad)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// OpenAEAD reverses SealAEAD; aad must match.
func OpenAEAD(key, blob, aad []byte) ([]byte, error) {
	if len(blob) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return nil, errors.New("envelope ciphertext too short")
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AEAD")
	}
	nonce := blob[:chacha20poly1305.NonceSize]
	ct := blob[chacha20poly1305.NonceSize:]
	pt, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt envelope value")
	}
	return pt, nil
}

// ValidEnvelopeBlob reports whether raw is a plausibly-shaped envelope value blob
// (nonce + at least the AEAD tag), for the offline lint gate.
func ValidEnvelopeBlob(raw []byte) error {
	if len(raw) < chacha20poly1305.NonceSize+chacha20poly1305.Overhead {
		return errors.Errorf("envelope value blob is %d bytes, below the %d-byte minimum", len(raw), chacha20poly1305.NonceSize+chacha20poly1305.Overhead)
	}
	return nil
}
