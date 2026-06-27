package ciphers

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"io"

	"filippo.io/edwards25519"
	"github.com/pkg/errors"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// x25519Magic marks a sealed-box blob produced by encryptWithX25519, so
// DecryptLargeStringWithEd25519 can route new blobs to the X25519 path while
// still reading legacy (pre-fix) blobs during migration. Legacy blobs began
// with a 32-byte random salt, so this fixed 8-byte prefix collides with a
// legacy blob with probability 2^-64.
var x25519Magic = []byte("scx25519")

const x25519Version byte = 1

// ed25519PublicKeyToX25519 maps an Ed25519 public key (an Edwards point) to the
// equivalent X25519 (Montgomery u-coordinate) public key — the standard
// birational map also used by age/agessh. Conversion is delegated to the vetted
// filippo.io/edwards25519 rather than hand-rolled.
func ed25519PublicKeyToX25519(pub ed25519.PublicKey) ([]byte, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, errors.Errorf("invalid ed25519 public key size: %d", len(pub))
	}
	p, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode ed25519 public key as an edwards point")
	}
	return p.BytesMontgomery(), nil
}

// ed25519PrivateKeyToX25519 maps an Ed25519 private key to the equivalent X25519
// private scalar: clamp(SHA-512(seed)[:32]) per RFC 8032 / RFC 7748. Clamping is
// idempotent; crypto/ecdh also clamps internally on use.
func ed25519PrivateKeyToX25519(priv ed25519.PrivateKey) []byte {
	h := sha512.Sum512(priv.Seed())
	s := h[:32]
	s[0] &= 248
	s[31] &= 127
	s[31] |= 64
	return s
}

// encryptWithX25519 seals plaintext to an Ed25519 recipient using ephemeral-static
// X25519 ECDH → HKDF-SHA256 → ChaCha20-Poly1305 (a sealed box). The AEAD key is
// derived from the ECDH *shared secret*, so only the recipient's private key can
// decrypt — unlike the removed legacy scheme, whose key was derived from the
// public key alone (and was therefore decryptable by anyone with read access).
//
// Blob layout: magic(8) | version(1) | ephPub(32) | nonce(12) | ciphertext.
func encryptWithX25519(recipientEd25519Pub ed25519.PublicKey, plaintext []byte) ([]byte, error) {
	xPub, err := ed25519PublicKeyToX25519(recipientEd25519Pub)
	if err != nil {
		return nil, err
	}
	curve := ecdh.X25519()
	recipientPub, err := curve.NewPublicKey(xPub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build recipient X25519 public key")
	}
	ephPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate ephemeral X25519 key")
	}
	ephPub := ephPriv.PublicKey().Bytes()
	shared, err := ephPriv.ECDH(recipientPub)
	if err != nil {
		return nil, errors.Wrap(err, "ephemeral ECDH failed")
	}
	key, err := deriveX25519Key(shared, ephPub, xPub)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AEAD")
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "failed to generate nonce")
	}
	ct := aead.Seal(nil, nonce, plaintext, x25519AAD(ephPub))

	blob := make([]byte, 0, len(x25519Magic)+1+len(ephPub)+len(nonce)+len(ct))
	blob = append(blob, x25519Magic...)
	blob = append(blob, x25519Version)
	blob = append(blob, ephPub...)
	blob = append(blob, nonce...)
	blob = append(blob, ct...)
	return blob, nil
}

// decryptWithX25519 reverses encryptWithX25519 using the recipient's Ed25519
// private key (converted to X25519).
func decryptWithX25519(recipientEd25519Priv ed25519.PrivateKey, blob []byte) ([]byte, error) {
	// ed25519.PrivateKey.Seed() panics on a wrong-sized key; fail safely instead.
	if len(recipientEd25519Priv) != ed25519.PrivateKeySize {
		return nil, errors.Errorf("invalid ed25519 private key size: %d", len(recipientEd25519Priv))
	}
	// Minimum = magic + version + ephPub + nonce + the 16-byte AEAD tag. Reject
	// undersized blobs before doing ECDH/HKDF so trivially-invalid input can't
	// burn CPU.
	headerLen := len(x25519Magic) + 1 + 32 + chacha20poly1305.NonceSize + chacha20poly1305.Overhead
	if len(blob) < headerLen {
		return nil, errors.New("x25519 ciphertext too short")
	}
	off := len(x25519Magic)
	if !bytes.Equal(blob[:off], x25519Magic) {
		return nil, errors.New("not an x25519 blob")
	}
	if blob[off] != x25519Version {
		return nil, errors.Errorf("unsupported x25519 blob version: %d", blob[off])
	}
	off++
	ephPub := blob[off : off+32]
	off += 32
	nonce := blob[off : off+chacha20poly1305.NonceSize]
	off += chacha20poly1305.NonceSize
	ct := blob[off:]

	curve := ecdh.X25519()
	xPriv, err := curve.NewPrivateKey(ed25519PrivateKeyToX25519(recipientEd25519Priv))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build recipient X25519 private key")
	}
	ephPubKey, err := curve.NewPublicKey(ephPub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ephemeral public key")
	}
	shared, err := xPriv.ECDH(ephPubKey)
	if err != nil {
		return nil, errors.Wrap(err, "ECDH failed")
	}
	key, err := deriveX25519Key(shared, ephPub, xPriv.PublicKey().Bytes())
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AEAD")
	}
	pt, err := aead.Open(nil, nonce, ct, x25519AAD(ephPub))
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt data")
	}
	return pt, nil
}

// deriveX25519Key derives the AEAD key from the ECDH shared secret, binding it to
// both public keys so a wrapped blob cannot be transplanted between recipients.
func deriveX25519Key(shared, ephPub, recipientXPub []byte) ([]byte, error) {
	info := make([]byte, 0, len(x25519Magic)+1+len(ephPub)+len(recipientXPub))
	info = append(info, x25519Magic...)
	info = append(info, x25519Version)
	info = append(info, ephPub...)
	info = append(info, recipientXPub...)
	r := hkdf.New(sha256.New, shared, nil, info)
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, errors.Wrap(err, "failed to derive key")
	}
	return key, nil
}

// x25519AAD binds the format magic, version, and ephemeral public key into the
// AEAD's associated data so none of them can be altered without failing Open.
func x25519AAD(ephPub []byte) []byte {
	aad := make([]byte, 0, len(x25519Magic)+1+len(ephPub))
	aad = append(aad, x25519Magic...)
	aad = append(aad, x25519Version)
	aad = append(aad, ephPub...)
	return aad
}

// isX25519Blob reports whether b was produced by encryptWithX25519.
func isX25519Blob(b []byte) bool {
	return len(b) >= len(x25519Magic) && bytes.Equal(b[:len(x25519Magic)], x25519Magic)
}
