package ciphers

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"testing"

	. "github.com/onsi/gomega"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

func TestX25519_RoundTrip(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	msg := []byte("super secret 🔐\nDB_PASSWORD=hunter2\nAPI_KEY=abc")
	blob, err := encryptWithX25519(pub, msg)
	Expect(err).ToNot(HaveOccurred())
	Expect(isX25519Blob(blob)).To(BeTrue())

	got, err := decryptWithX25519(priv, blob)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal(msg))
}

// TestX25519_KeyConversionConsistency proves the Ed25519->X25519 birational map:
// the X25519 public derived from the converted private must equal the X25519
// public converted directly from the Ed25519 public.
func TestX25519_KeyConversionConsistency(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	xPub, err := ed25519PublicKeyToX25519(pub)
	Expect(err).ToNot(HaveOccurred())

	xPriv, err := ecdh.X25519().NewPrivateKey(ed25519PrivateKeyToX25519(priv))
	Expect(err).ToNot(HaveOccurred())
	Expect(xPriv.PublicKey().Bytes()).To(Equal(xPub))
}

func TestX25519_WrongRecipientCannotDecrypt(t *testing.T) {
	RegisterTestingT(t)
	_, pubA, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	privB, _, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	blob, err := encryptWithX25519(pubA, []byte("for A only"))
	Expect(err).ToNot(HaveOccurred())
	_, err = decryptWithX25519(privB, blob)
	Expect(err).To(HaveOccurred())
}

func TestX25519_NonDeterministic(t *testing.T) {
	RegisterTestingT(t)
	_, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	a, err := encryptWithX25519(pub, []byte("same"))
	Expect(err).ToNot(HaveOccurred())
	b, err := encryptWithX25519(pub, []byte("same"))
	Expect(err).ToNot(HaveOccurred())
	Expect(a).NotTo(Equal(b)) // fresh ephemeral key each time
}

// --- security regression: the public-key-only decrypt vuln is fixed ---

// legacyBrokenEncrypt reproduces the REMOVED encryptWithEd25519: the AEAD key was
// derived from the recipient PUBLIC key + salt, with no key agreement.
func legacyBrokenEncrypt(pub []byte, pt []byte) []byte {
	salt := make([]byte, 32)
	_, _ = rand.Read(salt)
	r := hkdf.New(sha256.New, pub, salt, []byte("ed25519-chacha20poly1305"))
	key := make([]byte, 32)
	_, _ = io.ReadFull(r, key)
	aead, _ := chacha20poly1305.New(key)
	nonce := make([]byte, chacha20poly1305.NonceSize)
	_, _ = rand.Read(nonce)
	ct := aead.Seal(nil, nonce, pt, nil)
	return append(append(append([]byte{}, salt...), nonce...), ct...)
}

// publicOnlyDecryptLegacy mounts the original attack: derive the key from the
// PUBLIC key + the salt prefix, then open. Works on legacy blobs, must fail on
// X25519 blobs.
func publicOnlyDecryptLegacy(pub, blob []byte) ([]byte, error) {
	salt, nonce, ct := blob[0:32], blob[32:44], blob[44:]
	r := hkdf.New(sha256.New, pub, salt, []byte("ed25519-chacha20poly1305"))
	key := make([]byte, 32)
	_, _ = io.ReadFull(r, key)
	aead, _ := chacha20poly1305.New(key)
	return aead.Open(nil, nonce, ct, nil)
}

func TestX25519_FixesPublicKeyDecryptVuln(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	secret := []byte("DB_PASSWORD=hunter2")

	// 1) Legacy scheme leaked: decryptable with the PUBLIC key alone.
	legacy := legacyBrokenEncrypt(pub, secret)
	got, err := publicOnlyDecryptLegacy(pub, legacy)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal(secret))

	// 2) X25519 scheme: the same public-only attack must fail. Strip magic+ver so
	// the attack "sees" an ephPub|nonce|ct prefix shaped like the old salt|nonce.
	fixed, err := encryptWithX25519(pub, secret)
	Expect(err).ToNot(HaveOccurred())
	_, err = publicOnlyDecryptLegacy(pub, fixed[len(x25519Magic)+1:])
	Expect(err).To(HaveOccurred())

	// 3) The real private key still decrypts.
	got, err = decryptWithX25519(priv, fixed)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal(secret))
}

// --- migration back-compat: legacy blobs still readable through the router ---

func TestEd25519_LegacyBlobStillDecryptsViaRouter(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())
	secret := []byte("legacy migration value")

	chunk := base64.StdEncoding.EncodeToString(legacyBrokenEncrypt(pub, secret))
	got, err := DecryptLargeStringWithEd25519(priv, []string{chunk})
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(Equal(secret))
}

func TestEd25519_PublicAPIProducesX25519(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	chunks, err := EncryptLargeString(pub, "hello world")
	Expect(err).ToNot(HaveOccurred())
	Expect(chunks).To(HaveLen(1))

	raw, err := base64.StdEncoding.DecodeString(chunks[0])
	Expect(err).ToNot(HaveOccurred())
	Expect(isX25519Blob(raw)).To(BeTrue()) // new format, not the legacy scheme

	got, err := DecryptLargeStringWithEd25519(priv, chunks)
	Expect(err).ToNot(HaveOccurred())
	Expect(string(got)).To(Equal("hello world"))
}

func TestX25519_VersionByteIsRejectedWhenTampered(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	blob, err := encryptWithX25519(pub, []byte("secret"))
	Expect(err).ToNot(HaveOccurred())
	// Flip the version byte (immediately after the magic). It is bound into the
	// AEAD AAD + HKDF info and explicitly checked, so decrypt must reject it.
	blob[len(x25519Magic)] ^= 0xff
	_, err = decryptWithX25519(priv, blob)
	Expect(err).To(HaveOccurred())
}

func TestX25519_InvalidPrivateKeyLengthErrorsNotPanics(t *testing.T) {
	RegisterTestingT(t)
	priv, _, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	// A wrong-sized key would panic in ed25519.PrivateKey.Seed(); we must return
	// a clean error instead.
	_, err = decryptWithX25519(priv[:10], []byte("not-a-real-blob"))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("invalid ed25519 private key size"))
}

// TestX25519_TamperedEphemeralKeyFails makes the AAD/HKDF binding load-bearing:
// the ephemeral public key has no explicit equality check, so altering it can
// only be caught cryptographically (ECDH + HKDF info + AEAD AAD all change).
func TestX25519_TamperedEphemeralKeyFails(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	blob, err := encryptWithX25519(pub, []byte("secret value"))
	Expect(err).ToNot(HaveOccurred())
	// ephPub begins right after magic(8)+version(1).
	blob[len(x25519Magic)+1] ^= 0xff
	_, err = decryptWithX25519(priv, blob)
	Expect(err).To(HaveOccurred())
}

func TestX25519_EmptyPlaintextRoundTrip(t *testing.T) {
	RegisterTestingT(t)
	priv, pub, err := GenerateEd25519KeyPair()
	Expect(err).ToNot(HaveOccurred())

	blob, err := encryptWithX25519(pub, []byte{})
	Expect(err).ToNot(HaveOccurred())
	got, err := decryptWithX25519(priv, blob)
	Expect(err).ToNot(HaveOccurred())
	Expect(got).To(BeEmpty())
}
