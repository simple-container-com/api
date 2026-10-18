// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package ciphers

import (
	"encoding/base64"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// TestAAD_NilRoundTrip_LegacyCompatible proves a nil-AAD seal (the whole-file
// store's path) still round-trips, for both RSA and ed25519 recipients.
func TestAAD_NilRoundTrip_LegacyCompatible(t *testing.T) {
	RegisterTestingT(t)
	rpriv, rpub, err := GenerateKeyPair(2048)
	Expect(err).NotTo(HaveOccurred())
	chunks, err := EncryptLargeStringWithAAD(rpub, "legacy-value", nil)
	Expect(err).NotTo(HaveOccurred())
	got, err := DecryptLargeStringWithAAD(rpriv, chunks, nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(got)).To(Equal("legacy-value"))

	epriv, epub, err := GenerateEd25519KeyPair()
	Expect(err).NotTo(HaveOccurred())
	echunks, err := EncryptLargeStringWithAAD(epub, "legacy-ed", nil)
	Expect(err).NotTo(HaveOccurred())
	egot, err := DecryptLargeStringWithEd25519AAD(epriv, echunks, nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(egot)).To(Equal("legacy-ed"))
}

// TestAAD_MismatchFails is the core security invariant: a value sealed under a
// specific AAD must NOT decrypt under a different (or nil) AAD, for both schemes.
func TestAAD_MismatchFails(t *testing.T) {
	RegisterTestingT(t)
	ctx := []byte("sc-scope-v1\x00pr\x00k")
	other := []byte("sc-scope-v1\x00prod\x00k")

	// RSA (OAEP label)
	rpriv, rpub, err := GenerateKeyPair(2048)
	Expect(err).NotTo(HaveOccurred())
	rchunks, err := EncryptLargeStringWithAAD(rpub, "v", ctx)
	Expect(err).NotTo(HaveOccurred())
	_, err = DecryptLargeStringWithAAD(rpriv, rchunks, nil)
	Expect(err).To(HaveOccurred())
	_, err = DecryptLargeStringWithAAD(rpriv, rchunks, other)
	Expect(err).To(HaveOccurred())
	ok, err := DecryptLargeStringWithAAD(rpriv, rchunks, ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(ok)).To(Equal("v"))

	// ed25519 (AEAD associated data)
	epriv, epub, err := GenerateEd25519KeyPair()
	Expect(err).NotTo(HaveOccurred())
	echunks, err := EncryptLargeStringWithAAD(epub, "v", ctx)
	Expect(err).NotTo(HaveOccurred())
	_, err = DecryptLargeStringWithEd25519AAD(epriv, echunks, nil)
	Expect(err).To(HaveOccurred())
	_, err = DecryptLargeStringWithEd25519AAD(epriv, echunks, other)
	Expect(err).To(HaveOccurred())
	eok, err := DecryptLargeStringWithEd25519AAD(epriv, echunks, ctx)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(eok)).To(Equal("v"))
}

// TestRSAChunkFraming_ReorderTruncateSpliceFails covers P0-B: a multi-chunk RSA
// value bound with a non-nil AAD must reject reordered/dropped chunks.
func TestRSAChunkFraming_ReorderTruncateSpliceFails(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateKeyPair(2048)
	Expect(err).NotTo(HaveOccurred())
	aad := []byte("sc-scope-v1\x00prod\x00conn")
	long := strings.Repeat("A", 200) + strings.Repeat("B", 200) // >128B ⇒ multiple chunks
	chunks, err := EncryptLargeStringWithAAD(pub, long, aad)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(chunks)).To(BeNumerically(">=", 2), "value must span multiple RSA chunks")

	// baseline round-trip works
	got, err := DecryptLargeStringWithAAD(priv, chunks, aad)
	Expect(err).NotTo(HaveOccurred())
	Expect(string(got)).To(Equal(long))

	// reorder → fail (index binding)
	reordered := append([]string{}, chunks...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	_, err = DecryptLargeStringWithAAD(priv, reordered, aad)
	Expect(err).To(HaveOccurred())

	// truncate (drop last) → fail (count binding)
	_, err = DecryptLargeStringWithAAD(priv, chunks[:len(chunks)-1], aad)
	Expect(err).To(HaveOccurred())
}

// TestEd25519Downgrade_RejectedUnderAAD covers P0-A: a non-X25519 (legacy-shaped)
// blob must be refused when an AAD binding is expected, so a forged legacy blob
// cannot bypass the scope/key binding.
func TestEd25519Downgrade_RejectedUnderAAD(t *testing.T) {
	RegisterTestingT(t)
	priv, _, err := GenerateEd25519KeyPair()
	Expect(err).NotTo(HaveOccurred())
	// A blob without the scx25519 magic looks like the legacy format.
	fakeLegacy := base64.StdEncoding.EncodeToString(make([]byte, 60))
	_, err = DecryptLargeStringWithEd25519AAD(priv, []string{fakeLegacy}, []byte("sc-scope-v1\x00pr\x00k"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("legacy"))
}
