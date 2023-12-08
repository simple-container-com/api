package ciphers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"golang.org/x/crypto/ssh"
)

// GenerateKeyPair generates a new key pair
func GenerateKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privkey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}
	return privkey, &privkey.PublicKey, nil
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

func ParsePublicKey(s string) (*rsa.PublicKey, error) {
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s))
	if err != nil {
		return nil, err
	}

	if parsedCryptoKey, ok := parsed.(ssh.CryptoPublicKey); !ok {
		return nil, errors.New("failed to parse public key: not a CryptoPublicKey")
	} else if res, ok := parsedCryptoKey.CryptoPublicKey().(*rsa.PublicKey); !ok {
		return nil, errors.New("failed to parse public key: not a RSA public key")
	} else {
		return res, nil
	}
}

func EncryptLargeString(key *rsa.PublicKey, s string) ([]string, error) {
	chunks := lo.ChunkString(s, key.Size()/2)
	res := make([]string, len(chunks))
	for idx, chunk := range chunks {
		encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, key, []byte(chunk), nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to encrypt secret")
		}
		res[idx] = base64.StdEncoding.EncodeToString(encryptedData)
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
