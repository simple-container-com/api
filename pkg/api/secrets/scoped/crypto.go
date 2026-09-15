// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"

	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
)

// aadDomain domain-separates scoped-secret associated data from any other use of
// the underlying cipher. Bumping it is a breaking format change.
const aadDomain = "sc-scope-v1"

// valueAAD binds a sealed value to its (stack, scope, key) so a ciphertext blob
// cannot be transplanted into another scope file, another stack's file, or moved
// onto another key without failing AEAD/OAEP verification on decrypt. The NUL
// separators cannot appear in a stack/scope name or key (all validated to a
// restricted charset), so the concatenation is unambiguous. The KMS recipient path
// binds the same fields via a KMS EncryptionContext (see kmsEncryptionContext).
func valueAAD(stack, scope, key string) []byte {
	return []byte(aadDomain + "\x00" + stack + "\x00" + scope + "\x00" + key)
}

// recipientFingerprint returns the stable SHA256 SSH fingerprint of an authorized
// public key (e.g. "SHA256:abc…"). It is used as the per-recipient map key inside a
// scope file for SSH recipients: readable, order-independent, and derivable from a
// private key so a decryptor can find its own slot. KMS recipients use their
// normalized awskms:// URL instead — see recipientID.
//
// This funnels through the same ssh.ParseAuthorizedKey that ciphers.ParsePublicKey
// uses, so the fingerprint (identity) and the encryptability check
// (validateEncryptableRecipient, via ParsePublicKey) never disagree on which keys
// parse; the latter is the sole authority on whether a parsed key can actually
// receive a secret (rsa/ed25519 only), enforced at `allow` time.
func recipientFingerprint(authorizedKey string) (string, error) {
	pub, err := parseAuthorizedKey(authorizedKey)
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(pub), nil
}

// recipientID returns the stable wrap-slot identity for any recipient: the SHA256
// SSH fingerprint for an ssh-* key, or the normalized awskms:// URL for a KMS key.
// It is offline-derivable for both kinds, so recipient-drift lint and dedup work
// without a private key or a KMS call.
func recipientID(recipient string) (string, error) {
	if isKMSRecipient(recipient) {
		r, err := parseKMSRecipient(recipient)
		if err != nil {
			return "", err
		}
		return r.raw, nil
	}
	return recipientFingerprint(recipient)
}

// validateEncryptableRecipient rejects recipients that fingerprint fine but cannot
// actually receive a scoped secret. SSH recipients must be ssh-rsa or ssh-ed25519
// (the cipher layer supports no others); KMS recipients must be a well-formed
// awskms:// URL. Caught at governance time (`allow`) rather than failing later on
// the first `set`.
func validateEncryptableRecipient(recipient string) error {
	if isKMSRecipient(recipient) {
		return errors.Wrap(validateKMSRecipient(recipient), "unusable KMS recipient")
	}
	pub, err := ciphers.ParsePublicKey(secrets.TrimPubKey(recipient))
	if err != nil {
		return errors.Wrap(err, "unusable recipient key")
	}
	switch pub.(type) {
	case *rsa.PublicKey, ed25519.PublicKey:
		return nil
	default:
		return errors.Errorf("unsupported recipient key type %T (only ssh-rsa, ssh-ed25519, and awskms:// can receive scoped secrets)", pub)
	}
}

// parseAuthorizedKey parses one "<type> <data> [comment]" authorized-key line into
// an ssh.PublicKey.
func parseAuthorizedKey(authorizedKey string) (ssh.PublicKey, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse recipient public key")
	}
	return pub, nil
}

// encryptForRecipients builds the envelope for value: it is AEAD-encrypted ONCE
// under a fresh random data key (bound to stack/scope/key), and that data key is
// wrapped per recipient (keyed by recipient ID — SSH fingerprint or awskms:// URL).
// Because the value ciphertext is shared, every recipient decrypts the SAME
// plaintext — a tampered per-recipient slot yields a decrypt failure, never a
// different value — and the whole value is one AEAD blob (no chunk splicing). Every
// recipient must wrap successfully — a partial result is never returned. A KMS
// recipient's wrap calls kms:Encrypt, so the operator needs that permission.
func encryptForRecipients(recipients []string, stack, scope, key, value string) (EncryptedValue, error) {
	aad := valueAAD(stack, scope, key)
	dek, err := ciphers.GenerateDEK()
	if err != nil {
		return EncryptedValue{}, err
	}
	blob, err := ciphers.SealAEAD(dek, []byte(value), aad)
	if err != nil {
		return EncryptedValue{}, errors.Wrapf(err, "failed to seal value %q in scope %q", key, scope)
	}
	wraps := make(map[string][]string, len(recipients))
	for _, rk := range recipients {
		id, err := recipientID(rk)
		if err != nil {
			return EncryptedValue{}, err
		}
		if _, dup := wraps[id]; dup {
			return EncryptedValue{}, errors.Errorf("duplicate recipient %s in scope %q", id, scope)
		}
		var wrapped []string
		if isKMSRecipient(rk) {
			// The 32-byte DEK is wrapped by KMS; the (stack,scope,key) binding rides
			// as a KMS EncryptionContext, so the wrap cannot be moved to another value.
			wrapped, err = wrapDEKKMS(rk, dek, stack, scope, key)
		} else {
			cryptoPub, perr := ciphers.ParsePublicKey(secrets.TrimPubKey(rk))
			if perr != nil {
				return EncryptedValue{}, errors.Wrapf(perr, "failed to parse recipient %s", id)
			}
			// The DEK is 32 bytes → always a single RSA-OAEP block / X25519 box, so the
			// wrap is never chunked. The wrap is AAD-bound too, so it cannot be moved to
			// another (stack,scope,key).
			wrapped, err = ciphers.EncryptLargeStringWithAAD(cryptoPub, string(dek), aad)
		}
		if err != nil {
			return EncryptedValue{}, errors.Wrapf(err, "failed to wrap data key for recipient %s", id)
		}
		wraps[id] = wrapped
	}
	if len(wraps) == 0 {
		return EncryptedValue{}, errors.Errorf("scope %q has no recipients to encrypt %q for", scope, key)
	}
	return EncryptedValue{Ciphertext: base64.StdEncoding.EncodeToString(blob), Wraps: wraps}, nil
}

// privateKeyFingerprint parses an unencrypted PEM private key and returns its
// public SSH fingerprint plus the parsed key. Passphrase-protected keys are
// rejected with a clear error (CI scope keys are provisioned unencrypted).
func privateKeyFingerprint(privateKey string) (string, any, error) {
	raw, err := ssh.ParseRawPrivateKey([]byte(privateKey))
	if err != nil {
		if _, isMissing := err.(*ssh.PassphraseMissingError); isMissing {
			return "", nil, errors.New("scope private key is passphrase-protected; scoped CI keys must be unencrypted")
		}
		return "", nil, errors.Wrap(err, "failed to parse scope private key")
	}
	var sshPub ssh.PublicKey
	switch k := raw.(type) {
	case *rsa.PrivateKey:
		sshPub, err = ssh.NewPublicKey(&k.PublicKey)
	case ed25519.PrivateKey:
		sshPub, err = ssh.NewPublicKey(k.Public())
	case *ed25519.PrivateKey:
		sshPub, err = ssh.NewPublicKey(k.Public())
	default:
		return "", nil, errors.Errorf("unsupported private key type %T", raw)
	}
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to derive public key from scope private key")
	}
	return ssh.FingerprintSHA256(sshPub), raw, nil
}
