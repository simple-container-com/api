// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package ciphers

import (
	"crypto"
	"strings"
	"testing"

	"golang.org/x/crypto/ed25519"

	. "github.com/onsi/gomega"
)

func TestGenerateKeyPair(t *testing.T) {
	RegisterTestingT(t)

	t.Run("RSA 2048 key generation", func(t *testing.T) {
		privKey, pubKey, err := GenerateKeyPair(2048)
		Expect(err).To(BeNil())
		Expect(privKey).NotTo(BeNil())
		Expect(pubKey).NotTo(BeNil())
		Expect(privKey.N.BitLen()).To(Equal(2048))
	})

	t.Run("RSA 4096 key generation", func(t *testing.T) {
		privKey, pubKey, err := GenerateKeyPair(4096)
		Expect(err).To(BeNil())
		Expect(privKey).NotTo(BeNil())
		Expect(pubKey).NotTo(BeNil())
		Expect(privKey.N.BitLen()).To(Equal(4096))
	})
}

func TestGenerateEd25519KeyPair(t *testing.T) {
	RegisterTestingT(t)

	t.Run("ed25519 key generation", func(t *testing.T) {
		privKey, pubKey, err := GenerateEd25519KeyPair()
		Expect(err).To(BeNil())
		Expect(privKey).NotTo(BeNil())
		Expect(pubKey).NotTo(BeNil())
		Expect(len(privKey)).To(Equal(ed25519.PrivateKeySize))
		Expect(len(pubKey)).To(Equal(ed25519.PublicKeySize))
	})

	t.Run("ed25519 keys are different", func(t *testing.T) {
		privKey1, pubKey1, err := GenerateEd25519KeyPair()
		Expect(err).To(BeNil())
		privKey2, pubKey2, err := GenerateEd25519KeyPair()
		Expect(err).To(BeNil())

		Expect(privKey1).NotTo(Equal(privKey2))
		Expect(pubKey1).NotTo(Equal(pubKey2))
	})
}

func TestMarshalEd25519Keys(t *testing.T) {
	RegisterTestingT(t)

	privKey, pubKey, err := GenerateEd25519KeyPair()
	Expect(err).To(BeNil())

	t.Run("marshal ed25519 private key", func(t *testing.T) {
		pemKey, err := MarshalEd25519PrivateKey(privKey)
		Expect(err).To(BeNil())
		Expect(pemKey).To(ContainSubstring("-----BEGIN PRIVATE KEY-----"))
		Expect(pemKey).To(ContainSubstring("-----END PRIVATE KEY-----"))
	})

	t.Run("marshal ed25519 public key", func(t *testing.T) {
		sshKey, err := MarshalEd25519PublicKey(pubKey)
		Expect(err).To(BeNil())
		Expect(string(sshKey)).To(HavePrefix("ssh-ed25519 "))
	})
}

func TestRSAEncryptionDecryption(t *testing.T) {
	RegisterTestingT(t)

	privKey, pubKey, err := GenerateKeyPair(2048)
	Expect(err).To(BeNil())

	testData := "Hello, World! This is a test message for RSA encryption."

	t.Run("RSA encrypt/decrypt small message", func(t *testing.T) {
		encrypted, err := EncryptWithPublicRSAKey([]byte(testData), pubKey)
		Expect(err).To(BeNil())
		Expect(encrypted).NotTo(BeEmpty())

		decrypted, err := DecryptWithPrivateRSAKey(encrypted, privKey)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(testData))
	})

	t.Run("RSA encrypt/decrypt large string", func(t *testing.T) {
		largeData := strings.Repeat(testData, 20) // Create a large string

		encryptedChunks, err := EncryptLargeString(pubKey, largeData)
		Expect(err).To(BeNil())
		Expect(encryptedChunks).NotTo(BeEmpty())

		decrypted, err := DecryptLargeString(privKey, encryptedChunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(largeData))
	})
}

func TestEd25519EncryptionDecryption(t *testing.T) {
	RegisterTestingT(t)

	privKey, pubKey, err := GenerateEd25519KeyPair()
	Expect(err).To(BeNil())

	testData := "Hello, World! This is a test message for ed25519 encryption."

	t.Run("ed25519 encrypt/decrypt small message", func(t *testing.T) {
		encryptedChunks, err := EncryptLargeString(pubKey, testData)
		Expect(err).To(BeNil())
		Expect(encryptedChunks).To(HaveLen(1)) // ed25519 should return exactly one chunk

		decrypted, err := DecryptLargeStringWithEd25519(privKey, encryptedChunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(testData))
	})

	t.Run("ed25519 encrypt/decrypt large message", func(t *testing.T) {
		largeData := strings.Repeat(testData, 100) // Create a very large string

		encryptedChunks, err := EncryptLargeString(pubKey, largeData)
		Expect(err).To(BeNil())
		Expect(encryptedChunks).To(HaveLen(1)) // ed25519 should still return exactly one chunk

		decrypted, err := DecryptLargeStringWithEd25519(privKey, encryptedChunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(largeData))
	})

	t.Run("ed25519 encryption is non-deterministic", func(t *testing.T) {
		// Each encryption should produce different results due to ephemeral keys
		encrypted1, err := EncryptLargeString(pubKey, testData)
		Expect(err).To(BeNil())

		encrypted2, err := EncryptLargeString(pubKey, testData)
		Expect(err).To(BeNil())

		Expect(encrypted1[0]).NotTo(Equal(encrypted2[0]))
	})
}

func TestParsePublicKey(t *testing.T) {
	RegisterTestingT(t)

	t.Run("parse RSA public key", func(t *testing.T) {
		_, rsaPubKey, err := GenerateKeyPair(2048)
		Expect(err).To(BeNil())

		sshKey, err := MarshalPublicKey(rsaPubKey)
		Expect(err).To(BeNil())

		parsedKey, err := ParsePublicKey(string(sshKey))
		Expect(err).To(BeNil())
		Expect(parsedKey).NotTo(BeNil())
	})

	t.Run("parse ed25519 public key", func(t *testing.T) {
		_, ed25519PubKey, err := GenerateEd25519KeyPair()
		Expect(err).To(BeNil())

		sshKey, err := MarshalEd25519PublicKey(ed25519PubKey)
		Expect(err).To(BeNil())

		parsedKey, err := ParsePublicKey(string(sshKey))
		Expect(err).To(BeNil())
		Expect(parsedKey).NotTo(BeNil())

		// Verify it's actually an ed25519 key
		_, ok := parsedKey.(ed25519.PublicKey)
		Expect(ok).To(BeTrue())
	})

	t.Run("parse invalid public key", func(t *testing.T) {
		_, err := ParsePublicKey("invalid-key-data")
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("ssh: no key found"))
	})
}

func TestCrossKeyTypeCompatibility(t *testing.T) {
	RegisterTestingT(t)

	// Generate both RSA and ed25519 keys
	rsaPrivKey, rsaPubKey, err := GenerateKeyPair(2048)
	Expect(err).To(BeNil())

	ed25519PrivKey, ed25519PubKey, err := GenerateEd25519KeyPair()
	Expect(err).To(BeNil())

	testData := "Cross-compatibility test message"

	t.Run("EncryptLargeString detects RSA keys", func(t *testing.T) {
		encryptedChunks, err := EncryptLargeString(rsaPubKey, testData)
		Expect(err).To(BeNil())
		Expect(len(encryptedChunks)).To(BeNumerically(">", 0))

		decrypted, err := DecryptLargeString(rsaPrivKey, encryptedChunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(testData))
	})

	t.Run("EncryptLargeString detects ed25519 keys", func(t *testing.T) {
		encryptedChunks, err := EncryptLargeString(ed25519PubKey, testData)
		Expect(err).To(BeNil())
		Expect(encryptedChunks).To(HaveLen(1))

		decrypted, err := DecryptLargeStringWithEd25519(ed25519PrivKey, encryptedChunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(testData))
	})

	t.Run("unsupported key type", func(t *testing.T) {
		var unsupportedKey crypto.PublicKey = struct{}{}
		_, err := EncryptLargeString(unsupportedKey, testData)
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("unsupported key type"))
	})
}

func TestEd25519DecryptionEdgeCases(t *testing.T) {
	RegisterTestingT(t)

	privKey, _, err := GenerateEd25519KeyPair()
	Expect(err).To(BeNil())

	t.Run("decrypt with wrong number of chunks", func(t *testing.T) {
		// ed25519 expects exactly one chunk
		multipleChunks := []string{"chunk1", "chunk2"}
		_, err := DecryptLargeStringWithEd25519(privKey, multipleChunks)
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("expects exactly one chunk"))
	})

	t.Run("decrypt with invalid base64", func(t *testing.T) {
		invalidChunk := []string{"invalid-base64-data!!!"}
		_, err := DecryptLargeStringWithEd25519(privKey, invalidChunk)
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to decode base64"))
	})

	t.Run("decrypt with too short ciphertext", func(t *testing.T) {
		// Create valid base64 but too short for ed25519 format
		shortData := []string{"dGVzdA=="} // "test" in base64
		_, err := DecryptLargeStringWithEd25519(privKey, shortData)
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("ciphertext too short"))
	})
}

// TestEncryptLargeStringUTF8 guards against the rune-vs-byte chunking bug where
// multi-byte UTF-8 characters could push a chunk over the OAEP-SHA256 byte limit
// (190 bytes for a 2048-bit key), causing "crypto/rsa: message too long".
func TestEncryptLargeStringUTF8(t *testing.T) {
	RegisterTestingT(t)

	privKey, pubKey, err := GenerateKeyPair(2048)
	Expect(err).To(BeNil())

	t.Run("box-drawing flood: 120 runes × 3 bytes = 360 bytes", func(t *testing.T) {
		// U+2500 BOX DRAWINGS LIGHT HORIZONTAL encodes as 3 UTF-8 bytes.
		// 120 runes × 3 bytes = 360 bytes — far above the 190-byte OAEP limit.
		// Old rune-based chunking (128 runes/chunk) would pass 360 bytes to EncryptOAEP.
		s := strings.Repeat("─", 120)
		chunks, err := EncryptLargeString(pubKey, s)
		Expect(err).To(BeNil(), "encrypt must not fail for multi-byte UTF-8 input")

		decrypted, err := DecryptLargeString(privKey, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(s))
	})

	t.Run("mixed multi-byte characters > 190 bytes", func(t *testing.T) {
		// Mix of arrow (3 B), em-dash (3 B), Cyrillic (2 B), emoji (4 B)
		s := strings.Repeat("→—яя🔑", 20) // each repeat ~14 bytes; 20× = ~280 bytes
		chunks, err := EncryptLargeString(pubKey, s)
		Expect(err).To(BeNil())

		decrypted, err := DecryptLargeString(privKey, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(s))
	})

	t.Run("exactly 190 ASCII bytes produces 1 chunk", func(t *testing.T) {
		s := strings.Repeat("a", 190)
		chunks, err := EncryptLargeString(pubKey, s)
		Expect(err).To(BeNil())
		Expect(chunks).To(HaveLen(1))

		decrypted, err := DecryptLargeString(privKey, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(s))
	})

	t.Run("191 ASCII bytes splits into 2 chunks", func(t *testing.T) {
		s := strings.Repeat("a", 191)
		chunks, err := EncryptLargeString(pubKey, s)
		Expect(err).To(BeNil())
		Expect(chunks).To(HaveLen(2))

		decrypted, err := DecryptLargeString(privKey, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(s))
	})

	t.Run("empty string round-trip", func(t *testing.T) {
		chunks, err := EncryptLargeString(pubKey, "")
		Expect(err).To(BeNil())
		Expect(chunks).To(BeEmpty())

		decrypted, err := DecryptLargeString(privKey, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(""))
	})

	t.Run("4096-bit key round-trip with large UTF-8 payload", func(t *testing.T) {
		priv4096, pub4096, err := GenerateKeyPair(4096)
		Expect(err).To(BeNil())

		// maxPlain for 4096-bit key = 512 - 64 - 2 = 446 bytes
		s := strings.Repeat("─", 300) // 300 × 3 bytes = 900 bytes → 3 chunks
		chunks, err := EncryptLargeString(pub4096, s)
		Expect(err).To(BeNil())

		decrypted, err := DecryptLargeString(priv4096, chunks)
		Expect(err).To(BeNil())
		Expect(string(decrypted)).To(Equal(s))
	})
}

func TestKeyFormatting(t *testing.T) {
	RegisterTestingT(t)

	t.Run("RSA private key formatting", func(t *testing.T) {
		privKey, _, err := GenerateKeyPair(2048)
		Expect(err).To(BeNil())

		pemKey := MarshalRSAPrivateKey(privKey)
		Expect(pemKey).To(ContainSubstring("-----BEGIN RSA PRIVATE KEY-----"))
		Expect(pemKey).To(ContainSubstring("-----END RSA PRIVATE KEY-----"))
	})

	t.Run("RSA public key to SSH format", func(t *testing.T) {
		_, pubKey, err := GenerateKeyPair(2048)
		Expect(err).To(BeNil())

		sshKey, err := MarshalPublicKey(pubKey)
		Expect(err).To(BeNil())
		Expect(string(sshKey)).To(HavePrefix("ssh-rsa "))
	})

	t.Run("RSA public key to PEM format", func(t *testing.T) {
		_, pubKey, err := GenerateKeyPair(2048)
		Expect(err).To(BeNil())

		pemKey, err := PublicKeyToBytes(pubKey)
		Expect(err).To(BeNil())
		Expect(string(pemKey)).To(ContainSubstring("-----BEGIN RSA PUBLIC KEY-----"))
		Expect(string(pemKey)).To(ContainSubstring("-----END RSA PUBLIC KEY-----"))
	})
}
