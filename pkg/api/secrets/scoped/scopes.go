// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

// Package scoped implements the additive per-scope secret store
// (secrets.<scope>.yaml): committed-encrypted files whose values are sealed to a
// per-scope recipient set, so a pull_request-triggered CI job can be given a key
// that opens only the "pr" scope rather than the whole-file store. It reuses the
// sibling ciphers package (RSA-OAEP + X25519 sealed box); recipients are SSH
// public keys, values are bound to their (scope,key) via associated data. Old
// binaries never read these files (see the fail-closed schemaVersion guard on the
// legacy store); the whole-file store in the parent package is untouched.
package scoped

import (
	"os"
	"regexp"
	"sort"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// CurrentScopesSchemaVersion is the highest scopes.yaml / secrets.<scope>.yaml
// schema version this build understands. A store declaring a higher version is
// refused (fail-closed) — mirrors the whole-file store guard so a newer file is
// never read as empty and then clobbered.
const CurrentScopesSchemaVersion = 1

// ScopesFileName is the governance file at the .sc config root.
const ScopesFileName = "scopes.yaml"

var (
	// ErrRecipientNotAllowed indicates the decrypting key is not a recipient of a
	// value (as opposed to the value being corrupt or the wrong scope).
	ErrRecipientNotAllowed = errors.New("key is not a recipient of this scoped secret")
	// ErrScopesVersionUnsupported is fatal on every read path (fail-closed).
	ErrScopesVersionUnsupported = errors.New("unsupported scoped secrets schema version")

	// scopeNameRe / secretKeyRe restrict names to a NUL-free charset: the AAD
	// binding (valueAAD) uses NUL separators, and these are also used as filename
	// components, so control characters and separators are rejected.
	scopeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	secretKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

// ValidateScopeName rejects scope names that are unsafe as a filename component or
// AAD field.
func ValidateScopeName(scope string) error {
	if !scopeNameRe.MatchString(scope) {
		return errors.Errorf("invalid scope name %q (allowed: %s)", scope, scopeNameRe.String())
	}
	return nil
}

// ValidateSecretKey rejects secret keys that are unsafe as an AAD field.
func ValidateSecretKey(key string) error {
	if !secretKeyRe.MatchString(key) {
		return errors.Errorf("invalid secret key %q (allowed: %s)", key, secretKeyRe.String())
	}
	return nil
}

// Scopes is the .sc/scopes.yaml governance file: the authoritative per-scope
// recipient set. It is CODEOWNERS-gated at the repo level; `sc secrets lint`
// independently verifies each scope file's recipients against it so a scope file
// edited out-of-band (bypassing review) is still caught.
type Scopes struct {
	SchemaVersion int              `yaml:"schemaVersion"`
	Scopes        map[string]Scope `yaml:"scopes"`
}

// Scope is one named recipient set.
type Scope struct {
	Description string   `yaml:"description,omitempty"`
	Recipients  []string `yaml:"recipients"`
}

// LoadScopes reads and validates scopes.yaml. A missing file is not an error —
// it returns an empty, usable *Scopes (no scopes defined yet).
func LoadScopes(path string) (*Scopes, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Scopes{SchemaVersion: CurrentScopesSchemaVersion, Scopes: map[string]Scope{}}, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s", path)
	}
	var s Scopes
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s", path)
	}
	if s.SchemaVersion > CurrentScopesSchemaVersion {
		return nil, errors.Wrapf(ErrScopesVersionUnsupported, "%s declares version %d, this build supports up to %d", path, s.SchemaVersion, CurrentScopesSchemaVersion)
	}
	if s.Scopes == nil {
		s.Scopes = map[string]Scope{}
	}
	for name := range s.Scopes {
		if err := ValidateScopeName(name); err != nil {
			return nil, errors.Wrapf(err, "in %s", path)
		}
	}
	return &s, nil
}

// Save writes scopes.yaml with a stable field order.
func (s *Scopes) Save(path string) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = CurrentScopesSchemaVersion
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return errors.Wrap(err, "failed to marshal scopes")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errors.Wrapf(err, "failed to write %s", path)
	}
	return nil
}

// Recipients returns the authoritative recipient set for a scope, or an error if
// the scope is not declared (a value can only be encrypted to a governed set).
func (s *Scopes) Recipients(scope string) ([]string, error) {
	sc, ok := s.Scopes[scope]
	if !ok {
		return nil, errors.Errorf("scope %q is not declared in %s", scope, ScopesFileName)
	}
	if len(sc.Recipients) == 0 {
		return nil, errors.Errorf("scope %q has no recipients in %s", scope, ScopesFileName)
	}
	return sc.Recipients, nil
}

// Allow adds a recipient to a scope (creating the scope if needed) and returns
// whether it was newly added.
func (s *Scopes) Allow(scope, recipient string) (bool, error) {
	if err := ValidateScopeName(scope); err != nil {
		return false, err
	}
	if _, err := recipientFingerprint(recipient); err != nil {
		return false, err
	}
	if s.Scopes == nil {
		s.Scopes = map[string]Scope{}
	}
	sc := s.Scopes[scope]
	fp, _ := recipientFingerprint(recipient)
	for _, existing := range sc.Recipients {
		if efp, _ := recipientFingerprint(existing); efp == fp {
			return false, nil
		}
	}
	sc.Recipients = append(sc.Recipients, recipient)
	sort.Strings(sc.Recipients)
	s.Scopes[scope] = sc
	return true, nil
}

// Disallow removes a recipient from a scope and returns whether it was present.
// Removing a recipient does NOT revoke access to values already committed in
// history — callers must warn to rotate those values.
func (s *Scopes) Disallow(scope, recipient string) (bool, error) {
	sc, ok := s.Scopes[scope]
	if !ok {
		return false, errors.Errorf("scope %q is not declared", scope)
	}
	fp, err := recipientFingerprint(recipient)
	if err != nil {
		return false, err
	}
	kept := sc.Recipients[:0]
	removed := false
	for _, existing := range sc.Recipients {
		if efp, _ := recipientFingerprint(existing); efp == fp {
			removed = true
			continue
		}
		kept = append(kept, existing)
	}
	sc.Recipients = kept
	s.Scopes[scope] = sc
	return removed, nil
}
