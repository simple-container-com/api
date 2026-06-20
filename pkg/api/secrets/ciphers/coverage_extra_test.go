// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Simple Container

package ciphers

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// TestEncryptLargeString_RSAEncryptError covers the error branch inside
// EncryptLargeString's RSA path. EncryptLargeString chunks the plaintext at
// keySize/2 bytes, but OAEP-SHA256 only fits keySize - 2*32 - 2 bytes. For a
// 1024-bit key the chunk size (64) exceeds the OAEP budget (128 - 64 - 2 = 62),
// so rsa.EncryptOAEP fails on the over-sized chunk.
func TestEncryptLargeString_RSAEncryptError(t *testing.T) {
	RegisterTestingT(t)

	// 1024 is the smallest key size Go 1.26 will generate.
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	Expect(err).ToNot(HaveOccurred())

	// Plaintext must be long enough to produce a full 64-byte chunk.
	_, err = EncryptLargeString(&small.PublicKey, strings.Repeat("z", 64))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to encrypt secret"))
}

// TestDecryptLargeString_DecryptError covers DecryptLargeString's
// rsa.DecryptOAEP failure branch: the chunk is valid base64 of the correct
// ciphertext length, but is not a genuine OAEP ciphertext for this key.
func TestDecryptLargeString_DecryptError(t *testing.T) {
	RegisterTestingT(t)

	priv, _, err := GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())

	// 256 bytes == 2048-bit modulus size, so length checks pass but OAEP fails.
	garbage := make([]byte, 256)
	for i := range garbage {
		garbage[i] = 0x7f
	}
	chunk := base64.StdEncoding.EncodeToString(garbage)

	_, err = DecryptLargeString(priv, []string{chunk})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to decrypt secret"))
}

// TestDecryptLargeString_WrongKey covers the realistic case of decrypting a
// genuine ciphertext with the wrong RSA private key.
func TestDecryptLargeString_WrongKey(t *testing.T) {
	RegisterTestingT(t)

	_, pubA, err := GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())
	privB, _, err := GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())

	enc, err := EncryptLargeString(pubA, "secret payload that should not decrypt")
	Expect(err).ToNot(HaveOccurred())

	_, err = DecryptLargeString(privB, enc)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to decrypt secret"))
}

// TestEd25519_TamperedCiphertextFails covers decryptWithEd25519's AEAD-open
// failure branch: flipping a byte in the ciphertext breaks the Poly1305 tag.
func TestEd25519_TamperedCiphertextFails(t *testing.T) {
	RegisterTestingT(t)

	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	enc, err := EncryptLargeString(pub, "tamper me")
	Expect(err).ToNot(HaveOccurred())
	Expect(enc).To(HaveLen(1))

	raw, err := base64.StdEncoding.DecodeString(enc[0])
	Expect(err).ToNot(HaveOccurred())
	// Flip a byte in the AEAD ciphertext region (past salt+nonce).
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = DecryptLargeStringWithEd25519(priv, []string{tampered})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to decrypt data"))
}

// TestEd25519_WrongKeyFails covers decrypting a genuine ed25519 envelope with a
// different private key (HKDF derives a different AEAD key -> Open fails).
func TestEd25519_WrongKeyFails(t *testing.T) {
	RegisterTestingT(t)

	_, pubA, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	privB, _, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	enc, err := EncryptLargeString(pubA, "for A only")
	Expect(err).ToNot(HaveOccurred())

	_, err = DecryptLargeStringWithEd25519(privB, enc)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to decrypt data"))
}

// TestDecryptLargeStringWithEd25519_TruncatedAfterHeader covers the AEAD-open
// failure branch when the header (salt+nonce) is intact but the ciphertext body
// is too short to carry a valid Poly1305 tag.
func TestDecryptLargeStringWithEd25519_TruncatedAfterHeader(t *testing.T) {
	RegisterTestingT(t)

	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	enc, err := EncryptLargeString(pub, "some content")
	Expect(err).ToNot(HaveOccurred())
	raw, err := base64.StdEncoding.DecodeString(enc[0])
	Expect(err).ToNot(HaveOccurred())

	// Keep salt(32)+nonce(12) and a single body byte: passes the length guard
	// (>= 44) but cannot hold a 16-byte AEAD tag, so Open fails.
	truncated := base64.StdEncoding.EncodeToString(raw[:45])
	_, err = DecryptLargeStringWithEd25519(priv, []string{truncated})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to decrypt data"))
}

// TestParsePublicKey_RejectsNonAuthorizedKey covers ParsePublicKey's parse
// failure branch with input that is not an SSH authorized_keys line.
func TestParsePublicKey_RejectsNonAuthorizedKey(t *testing.T) {
	RegisterTestingT(t)

	_, err := ParsePublicKey(strings.Repeat("x", 64))
	Expect(err).To(HaveOccurred())
}

// TestParsePublicKey_RejectsCertificate covers ParsePublicKey's
// "not a CryptoPublicKey" branch: an OpenSSH certificate parses as an
// ssh.PublicKey but does not implement ssh.CryptoPublicKey.
func TestParsePublicKey_RejectsCertificate(t *testing.T) {
	RegisterTestingT(t)

	_, edPub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	subjectKey, err := ssh.NewPublicKey(edPub)
	Expect(err).ToNot(HaveOccurred())

	caPriv, _, err := GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())
	caSigner, err := ssh.NewSignerFromKey(caPriv)
	Expect(err).ToNot(HaveOccurred())

	cert := &ssh.Certificate{
		Key:         subjectKey,
		Serial:      1,
		CertType:    ssh.UserCert,
		KeyId:       "coverage",
		ValidBefore: ssh.CertTimeInfinity,
	}
	Expect(cert.SignCert(rand.Reader, caSigner)).To(Succeed())

	authLine := string(ssh.MarshalAuthorizedKey(cert))
	_, err = ParsePublicKey(authLine)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("not a CryptoPublicKey"))
}

// TestMarshalEd25519PublicKey_BadLength covers MarshalEd25519PublicKey's error
// branch: ssh.NewPublicKey rejects an ed25519 public key of the wrong size.
func TestMarshalEd25519PublicKey_BadLength(t *testing.T) {
	RegisterTestingT(t)

	bad := ed25519.PublicKey([]byte{1, 2, 3})
	_, err := MarshalEd25519PublicKey(bad)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid size"))
}

// TestEncryptLargeString_EmptyStringRSA pins the actual behavior for an empty
// plaintext on the RSA path: lo.ChunkString("", n) yields a single empty chunk,
// so encryption produces exactly one ciphertext chunk that round-trips back to
// the empty string (it does NOT short-circuit to an empty slice).
func TestEncryptLargeString_EmptyStringRSA(t *testing.T) {
	RegisterTestingT(t)

	priv, pub, err := GenerateKeyPair(2048)
	Expect(err).ToNot(HaveOccurred())

	enc, err := EncryptLargeString(pub, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(enc).To(HaveLen(1))

	dec, err := DecryptLargeString(priv, enc)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(dec)).To(Equal(""))
}
