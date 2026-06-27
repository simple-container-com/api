// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package ciphers

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/ssh"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// GenerateKeyPair generates a new RSA key pair
func GenerateKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privkey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}
	return privkey, &privkey.PublicKey, nil
}

// GenerateEd25519KeyPair generates a new ed25519 key pair
func GenerateEd25519KeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

// PrivateKeyToBytes private key to bytes
func PrivateKeyToBytes(priv *rsa.PrivateKey) []byte {
	privBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		},
	)

	return privBytes
}

func MarshalPublicKey(pub *rsa.PublicKey) ([]byte, error) {
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	sshPubBytes := sshPub.Marshal()

	// Now we can convert it back to PEM format
	// Remember: if you're reading the public key from a file, you probably
	// want ssh.ParseAuthorizedKey.
	sshKey, err := ssh.ParsePublicKey(sshPubBytes)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(sshKey), nil
}

// PublicKeyToBytes public key to bytes
func PublicKeyToBytes(pub *rsa.PublicKey) ([]byte, error) {
	pubASN1, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pubASN1,
	})

	return pubBytes, nil
}

// EncryptWithPublicRSAKey encrypts data with public key
func EncryptWithPublicRSAKey(msg []byte, pub *rsa.PublicKey) ([]byte, error) {
	hash := sha512.New()
	ciphertext, err := rsa.EncryptOAEP(hash, rand.Reader, pub, msg, nil)
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

// DecryptWithPrivateRSAKey decrypts data with private key
func DecryptWithPrivateRSAKey(ciphertext []byte, priv *rsa.PrivateKey) ([]byte, error) {
	hash := sha512.New()
	plaintext, err := rsa.DecryptOAEP(hash, rand.Reader, priv, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func MarshalRSAPrivateKey(priv *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv),
	}))
}

// MarshalEd25519PrivateKey marshals an ed25519 private key to PEM format
func MarshalEd25519PrivateKey(priv ed25519.PrivateKey) (string, error) {
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})), nil
}

// MarshalEd25519PublicKey marshals an ed25519 public key to SSH authorized key format
func MarshalEd25519PublicKey(pub ed25519.PublicKey) ([]byte, error) {
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return ssh.MarshalAuthorizedKey(sshPub), nil
}

func ParsePublicKey(s string) (crypto.PublicKey, error) {
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
	if err != nil {
		return nil, err
	}

	if parsedCryptoKey, ok := parsed.(ssh.CryptoPublicKey); !ok {
		return nil, errors.New("failed to parse public key: not a CryptoPublicKey")
	} else if res, ok := parsedCryptoKey.CryptoPublicKey().(crypto.PublicKey); !ok { //nolint: gosimple
		return nil, errors.New("failed to parse public key: not a supported public key type")
	} else {
		return res, nil
	}
}

func EncryptLargeString(key crypto.PublicKey, s string) ([]string, error) {
	var res []string
	if rsaKey, ok := key.(*rsa.PublicKey); ok {
		chunks := lo.ChunkString(s, rsaKey.Size()/2)
		res = make([]string, len(chunks))
		for idx, chunk := range chunks {
			encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaKey, []byte(chunk), nil)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to encrypt secret")
			}
			res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
		}
	} else if ed25519Key, ok := key.(ed25519.PublicKey); ok {
		// ed25519 recipients are sealed via ephemeral-static X25519 ECDH
		// (see x25519.go): the AEAD key is derived from the ECDH shared secret,
		// so only the holder of the private key can decrypt. The previous scheme
		// derived the key from the public key alone and is no longer produced.
		encryptedData, err := encryptWithX25519(ed25519Key, []byte(s))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encrypt secret for ed25519 recipient")
		}
		res = []string{base64.StdEncoding.EncodeToString(encryptedData)}
	} else {
		return nil, errors.New("unsupported key type for encryption")
	}
	return res, nil
}

func DecryptLargeString(key *rsa.PrivateKey, chunks []string) ([]byte, error) {
	decrChunks := make([][]byte, len(chunks))
	for idx, chunk := range chunks {
		chunkBytes, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode base64 string")
		}
		decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, chunkBytes, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decrypt secret")
		}
		decrChunks[idx] = decrypted
	}
	return []byte(strings.Join(lo.Map(decrChunks, func(chunk []byte, _ int) string {
		return string(chunk)
	}), "")), nil
}

// DecryptLargeStringWithEd25519 decrypts data encrypted with ed25519 hybrid encryption
func DecryptLargeStringWithEd25519(key ed25519.PrivateKey, chunks []string) ([]byte, error) {
	if len(chunks) != 1 {
		return nil, errors.New("ed25519 decryption expects exactly one chunk")
	}
	chunkBytes, err := base64.StdEncoding.DecodeString(chunks[0])
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode base64 string")
	}
	// New blobs use the X25519 sealed box (see x25519.go). Legacy blobs predate
	// it and are read-only here solely to support migration/re-encryption — they
	// are NOT confidential (the key was derived from public data); rotate any
	// secret ever stored in one.
	if isX25519Blob(chunkBytes) {
		return decryptWithX25519(key, chunkBytes)
	}
	return decryptWithEd25519(key, chunkBytes)
}

// NOTE: the former encryptWithEd25519 was removed. It derived the AEAD key from
// the recipient's PUBLIC key (HKDF(publicKey, salt)) with no key agreement, so
// the ciphertext was decryptable by anyone holding the public key — i.e. zero
// confidentiality. ed25519 recipients are now sealed via X25519 ECDH (x25519.go).

// decryptWithEd25519 reads a LEGACY (pre-X25519) ed25519 blob. Kept only so old
// stores can be decrypted for migration/re-encryption. Such blobs are NOT
// confidential — rotate any secret ever stored in one. New blobs use X25519.
func decryptWithEd25519(privateKey ed25519.PrivateKey, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 32+chacha20poly1305.NonceSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract components
	salt := ciphertext[0:32]
	nonce := ciphertext[32 : 32+chacha20poly1305.NonceSize]
	encryptedData := ciphertext[32+chacha20poly1305.NonceSize:]

	// Derive the public key from the private key for HKDF
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Use HKDF to derive the same encryption key using the public key and salt
	hkdfReader := hkdf.New(sha256.New, publicKey, salt, []byte("ed25519-chacha20poly1305"))
	encryptionKey := make([]byte, 32)
	if _, err := hkdfReader.Read(encryptionKey); err != nil {
		return nil, errors.Wrap(err, "failed to derive encryption key")
	}

	// Create ChaCha20-Poly1305 cipher
	cipher, err := chacha20poly1305.New(encryptionKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cipher")
	}

	// Decrypt the data
	plaintext, err := cipher.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decrypt data")
	}

	return plaintext, nil
}
