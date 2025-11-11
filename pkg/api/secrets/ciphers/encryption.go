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
		// For ed25519, use hybrid encryption with Curve25519 + ChaCha20-Poly1305
		encryptedData, err := encryptWithEd25519(ed25519Key, []byte(s))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encrypt secret with ed25519")
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
	return decryptWithEd25519(key, chunkBytes)
}

// encryptWithEd25519 performs hybrid encryption using HKDF key derivation and ChaCha20-Poly1305
func encryptWithEd25519(publicKey ed25519.PublicKey, plaintext []byte) ([]byte, error) {
	// Generate a random salt for HKDF
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Wrap(err, "failed to generate salt")
	}

	// Use HKDF to derive encryption key from ed25519 public key and salt
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

	// Generate nonce
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, errors.Wrap(err, "failed to generate nonce")
	}

	// Encrypt the plaintext
	ciphertext := cipher.Seal(nil, nonce, plaintext, nil)

	// Combine salt + nonce + ciphertext
	result := make([]byte, 32+len(nonce)+len(ciphertext))
	copy(result[0:32], salt)
	copy(result[32:32+len(nonce)], nonce)
	copy(result[32+len(nonce):], ciphertext)

	return result, nil
}

// decryptWithEd25519 performs hybrid decryption using HKDF key derivation and ChaCha20-Poly1305
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
