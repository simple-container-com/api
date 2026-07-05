// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"crypto"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/simple-container-com/api/pkg/api/secrets"
	"github.com/simple-container-com/api/pkg/api/secrets/ciphers"
)

// scopeFilePrefix / scopeFileSuffix bracket the scope name in a scope file's base
// name: secrets.<scope>.yaml.
const (
	scopeFilePrefix = "secrets."
	scopeFileSuffix = ".yaml"
)

// EncryptedValue is one secret in envelope form: the value is AEAD-encrypted ONCE
// under a random data key (Ciphertext), and that data key is wrapped per recipient
// (Wraps, keyed by recipient ID — a SHA256 SSH fingerprint for an ssh key, or a
// normalized awskms:// URL for a KMS recipient). All recipients therefore decrypt
// the SAME value — a tampered slot yields a decrypt failure, never a different
// plaintext — and the value carries a single whole-message MAC (no chunk splicing).
// Committed as-is (opaque values, diffable structure).
type EncryptedValue struct {
	Ciphertext string              `yaml:"ciphertext"`
	Wraps      map[string][]string `yaml:"wraps"`
}

// ScopeFile is a committed, encrypted secrets.<scope>.yaml. Structure (keys,
// recipients) is readable; values are opaque. It is self-contained: the recipient
// list lets `sc secrets lint` verify the value fingerprints without a private key.
type ScopeFile struct {
	SchemaVersion int                       `yaml:"schemaVersion"`
	Stack         string                    `yaml:"stack"`
	Scope         string                    `yaml:"scope"`
	Recipients    []string                  `yaml:"recipients"`
	Values        map[string]EncryptedValue `yaml:"values"`
}

// ScopeFileName returns the base name for a scope: secrets.<scope>.yaml.
func ScopeFileName(scope string) string {
	return scopeFilePrefix + scope + scopeFileSuffix
}

// ScopeNameFromFile extracts the scope from a scope file's path, or "" if the base
// name is not secrets.<scope>.yaml.
func ScopeNameFromFile(path string) string {
	base := filepath.Base(path)
	if len(base) <= len(scopeFilePrefix)+len(scopeFileSuffix) {
		return ""
	}
	if base[:len(scopeFilePrefix)] != scopeFilePrefix || base[len(base)-len(scopeFileSuffix):] != scopeFileSuffix {
		return ""
	}
	return base[len(scopeFilePrefix) : len(base)-len(scopeFileSuffix)]
}

// StackNameFromFile returns the stack a scope file belongs to — the name of its
// parent directory (.sc/stacks/<stack>/secrets.<scope>.yaml). Used to verify the
// file's self-declared stack against its actual location.
func StackNameFromFile(path string) string {
	return filepath.Base(filepath.Dir(path))
}

// NewScopeFile creates an empty scope file bound to a stack, a scope, and its
// recipient set. The stack + scope are bound into every value's AAD, so a value
// cannot be transplanted to another stack or scope.
func NewScopeFile(stack, scope string, recipients []string) (*ScopeFile, error) {
	if err := ValidateScopeName(scope); err != nil {
		return nil, err
	}
	if strings.TrimSpace(stack) == "" {
		return nil, errors.New("cannot create a scope file with no stack")
	}
	if len(recipients) == 0 {
		return nil, errors.Errorf("cannot create scope %q with no recipients", scope)
	}
	return &ScopeFile{
		SchemaVersion: CurrentScopesSchemaVersion,
		Stack:         stack,
		Scope:         scope,
		Recipients:    append([]string(nil), recipients...),
		Values:        map[string]EncryptedValue{},
	}, nil
}

// LoadScopeFile reads a scope file, fails closed on a too-new version, and verifies
// the in-file scope name matches the filename AND the in-file stack matches the
// parent directory — so copying secrets.prod.yaml into another stack's dir, or
// renaming it to another scope, is rejected here even before the per-value AAD
// binding would fail on decrypt.
func LoadScopeFile(path string) (*ScopeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read scope file %s", path)
	}
	var f ScopeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, errors.Wrapf(err, "failed to parse scope file %s", path)
	}
	if f.SchemaVersion > CurrentScopesSchemaVersion {
		return nil, errors.Wrapf(ErrScopesVersionUnsupported, "%s declares version %d, this build supports up to %d", path, f.SchemaVersion, CurrentScopesSchemaVersion)
	}
	if err := ValidateScopeName(f.Scope); err != nil {
		return nil, errors.Wrapf(err, "in %s", path)
	}
	if fromName := ScopeNameFromFile(path); fromName != "" && fromName != f.Scope {
		return nil, errors.Errorf("scope file %s declares scope %q but its filename says %q (renamed file?)", path, f.Scope, fromName)
	}
	if f.Stack == "" {
		return nil, errors.Errorf("scope file %s has no stack field", path)
	}
	if fromDir := StackNameFromFile(path); fromDir != f.Stack {
		return nil, errors.Errorf("scope file %s declares stack %q but lives under stack dir %q (moved file?)", path, f.Stack, fromDir)
	}
	if f.Values == nil {
		f.Values = map[string]EncryptedValue{}
	}
	return &f, nil
}

// Save writes the scope file with a stable key/recipient order for clean diffs.
func (f *ScopeFile) Save(path string) error {
	if f.SchemaVersion == 0 {
		f.SchemaVersion = CurrentScopesSchemaVersion
	}
	sort.Strings(f.Recipients)
	data, err := yaml.Marshal(f)
	if err != nil {
		return errors.Wrap(err, "failed to marshal scope file")
	}
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return errors.Wrapf(err, "failed to write scope file %s", path)
	}
	return nil
}

// Set encrypts value under key to every recipient of this scope file. It fails if
// the file has no recipients, so a value is never written unencrypted or to an
// empty audience.
func (f *ScopeFile) Set(key, value string) error {
	if err := ValidateSecretKey(key); err != nil {
		return err
	}
	if len(f.Recipients) == 0 {
		return errors.Errorf("scope file %q has no recipients", f.Scope)
	}
	enc, err := encryptForRecipients(f.Recipients, f.Stack, f.Scope, key, value)
	if err != nil {
		return err
	}
	if f.Values == nil {
		f.Values = map[string]EncryptedValue{}
	}
	f.Values[key] = enc
	return nil
}

// Get decrypts key with privateKey (an unencrypted PEM SSH private key), verifying
// the (stack,scope,key) binding. Returns ErrRecipientNotAllowed (wrapped) if the
// key is not a recipient, and a distinct not-found error if the key is absent. This
// is the SSH-only path used for resealing (allow/disallow); the CLI `get`/`doctor`
// and deploy-time resolution use Open, which also tries KMS.
func (f *ScopeFile) Get(key, privateKey string) (string, error) {
	enc, ok := f.Values[key]
	if !ok {
		return "", errors.Errorf("secret %q not found in scope %q", key, f.Scope)
	}
	// Surface clear parse errors (e.g. passphrase-protected) rather than a bare
	// "not a recipient".
	if _, _, err := privateKeyFingerprint(privateKey); err != nil {
		return "", err
	}
	val, owned, err := NewOpener([]string{privateKey}, false).OpenValue(f.Stack, f.Scope, key, enc)
	if err != nil {
		return "", err
	}
	if !owned {
		return "", errors.Wrapf(ErrRecipientNotAllowed, "key is not a recipient of %q in scope %q", key, f.Scope)
	}
	return val, nil
}

// Open decrypts key using an Opener, which may hold several SSH keys and/or permit
// KMS Decrypt via the ambient AWS credentials. It returns (value, true, nil) on
// success, ("", false, nil) if the opener is not a usable recipient (least-privilege
// skip), and ("", true, err) on an integrity failure. It is the path used by the
// CLI and by deploy-time resolution.
func (f *ScopeFile) Open(key string, o *Opener) (string, bool, error) {
	enc, ok := f.Values[key]
	if !ok {
		return "", false, errors.Errorf("secret %q not found in scope %q", key, f.Scope)
	}
	return o.OpenValue(f.Stack, f.Scope, key, enc)
}

// Delete removes a key. Returns whether it was present.
func (f *ScopeFile) Delete(key string) bool {
	if _, ok := f.Values[key]; !ok {
		return false
	}
	delete(f.Values, key)
	return true
}

// Keys returns the secret names in sorted order.
func (f *ScopeFile) Keys() []string {
	keys := make([]string, 0, len(f.Values))
	for k := range f.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Reencrypt re-seals every value to newRecipients, using opener (which must hold a
// CURRENT recipient — an SSH key or KMS access) to decrypt each value first. Used by
// allow/disallow to roll the recipient set. Because it takes an Opener, a scope whose
// only current recipient is a KMS key can still be resealed by an operator with
// kms:Decrypt. It is all-or-nothing: if any value cannot be decrypted or re-sealed the
// file is left untouched, so a partial reseal never drops a recipient's access
// silently. Note: this does NOT rewrite git history — a removed recipient can still
// read prior committed versions, so callers must warn to rotate values on removal.
func (f *ScopeFile) Reencrypt(newRecipients []string, opener *Opener) error {
	if len(newRecipients) == 0 {
		return errors.Errorf("refusing to reseal scope %q to an empty recipient set", f.Scope)
	}
	// Decrypt everything first against the current recipient set.
	plain := make(map[string]string, len(f.Values))
	for key := range f.Values {
		v, owned, err := f.Open(key, opener)
		if err != nil {
			return errors.Wrapf(err, "cannot reseal scope %q: failed to decrypt %q", f.Scope, key)
		}
		if !owned {
			return errors.Errorf("cannot reseal scope %q: the provided key/credentials are not a current recipient of %q", f.Scope, key)
		}
		plain[key] = v
	}
	// Build the new sealed set in a scratch file so a mid-way error can't corrupt f.
	next := &ScopeFile{SchemaVersion: f.SchemaVersion, Stack: f.Stack, Scope: f.Scope, Recipients: append([]string(nil), newRecipients...), Values: map[string]EncryptedValue{}}
	for key, val := range plain {
		if err := next.Set(key, val); err != nil {
			return errors.Wrapf(err, "cannot reseal scope %q: failed to re-encrypt %q", f.Scope, key)
		}
	}
	f.Recipients = next.Recipients
	f.Values = next.Values
	return nil
}

// VerifyConsistency is the offline (no private key) integrity check `sc secrets
// lint` runs: every value must be sealed to exactly the declared recipient set,
// keys/scope must be well-formed. It does NOT verify recipients against
// scopes.yaml — the caller does that so the drift error can name scopes.yaml.
func (f *ScopeFile) VerifyConsistency() error {
	if err := ValidateScopeName(f.Scope); err != nil {
		return err
	}
	if strings.TrimSpace(f.Stack) == "" {
		return errors.Errorf("scope %q has no stack", f.Scope)
	}
	if len(f.Recipients) == 0 {
		return errors.Errorf("scope %q has no recipients", f.Scope)
	}
	// Map each recipient ID to how its wrap must be shaped: an SSH recipient's wrap
	// is a block of its key type (checkable via its public key); a KMS recipient's
	// wrap is an opaque KMS ciphertext blob (checkable only for base64 + a minimum
	// length that excludes a plaintext data key).
	sshPub := make(map[string]crypto.PublicKey, len(f.Recipients))
	kmsIDs := make(map[string]struct{}, len(f.Recipients))
	for _, r := range f.Recipients {
		id, err := recipientID(r)
		if err != nil {
			return err
		}
		if isKMSRecipient(r) {
			if err := validateKMSRecipient(r); err != nil {
				return errors.Wrapf(err, "scope %q recipient %s", f.Scope, id)
			}
			kmsIDs[id] = struct{}{}
			continue
		}
		pub, err := ciphers.ParsePublicKey(secrets.TrimPubKey(r))
		if err != nil {
			return errors.Wrapf(err, "scope %q recipient %s", f.Scope, id)
		}
		sshPub[id] = pub
	}
	nRecipients := len(sshPub) + len(kmsIDs)
	for key, ev := range f.Values {
		if err := ValidateSecretKey(key); err != nil {
			return err
		}
		// The value blob: base64, and a plausibly-shaped envelope AEAD (nonce+tag),
		// so a hand-edited plaintext value is rejected offline without a key.
		blob, err := base64.StdEncoding.DecodeString(ev.Ciphertext)
		if err != nil {
			return errors.Wrapf(err, "scope %q key %q value ciphertext is not base64 (plaintext leak?)", f.Scope, key)
		}
		if err := ciphers.ValidEnvelopeBlob(blob); err != nil {
			return errors.Wrapf(err, "scope %q key %q value ciphertext", f.Scope, key)
		}
		if len(ev.Wraps) != nRecipients {
			return errors.Errorf("scope %q key %q data key wrapped for %d recipients, expected %d", f.Scope, key, len(ev.Wraps), nRecipients)
		}
		for id, chunks := range ev.Wraps {
			if len(chunks) == 0 {
				return errors.Errorf("scope %q key %q has an empty data-key wrap for recipient %s", f.Scope, key, id)
			}
			if _, isKMS := kmsIDs[id]; isKMS {
				// A KMS wrap is a single opaque ciphertext blob; verify base64 + a
				// minimum length so a plaintext data key smuggled into a KMS slot fails
				// offline (its EncryptionContext binding is enforced by KMS at decrypt).
				if len(chunks) != 1 {
					return errors.Errorf("scope %q key %q KMS wrap for %s must be exactly one blob, got %d", f.Scope, key, id, len(chunks))
				}
				raw, err := base64.StdEncoding.DecodeString(chunks[0])
				if err != nil {
					return errors.Wrapf(err, "scope %q key %q KMS wrap for %s is not base64", f.Scope, key, id)
				}
				if len(raw) < kmsMinCiphertextLen {
					return errors.Errorf("scope %q key %q KMS wrap for %s is %d bytes, below the %d-byte minimum (plaintext?)", f.Scope, key, id, len(raw), kmsMinCiphertextLen)
				}
				continue
			}
			pub, ok := sshPub[id]
			if !ok {
				return errors.Errorf("scope %q key %q wrapped for unknown recipient %s (not in recipients list)", f.Scope, key, id)
			}
			// Each SSH wrap is the 32-byte DEK sealed to the recipient — a single block
			// of the recipient's key type (RSA modulus size, or an X25519 sealed box).
			for i, chunk := range chunks {
				raw, err := base64.StdEncoding.DecodeString(chunk)
				if err != nil {
					return errors.Wrapf(err, "scope %q key %q recipient %s wrap %d is not base64", f.Scope, key, id, i)
				}
				if err := ciphers.ValidateCiphertextShape(pub, raw); err != nil {
					return errors.Wrapf(err, "scope %q key %q recipient %s wrap %d", f.Scope, key, id, i)
				}
			}
		}
	}
	return nil
}

// String renders a short human summary (scope + counts), never values.
func (f *ScopeFile) String() string {
	return fmt.Sprintf("scope %q: %d value(s), %d recipient(s)", f.Scope, len(f.Values), len(f.Recipients))
}
