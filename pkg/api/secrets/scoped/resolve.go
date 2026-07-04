// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// ErrScopedIntegrity tags a scoped-resolution failure that must NEVER be treated
// as "secrets simply absent": a value the key is a recipient of but cannot decrypt
// (tamper), a corrupt or renamed scope file, or the same key present in two
// openable scopes (ambiguous). Callers that otherwise tolerate a missing legacy
// secrets.yaml (IgnoreSecretsMissing) MUST still fail on errors.Is(err, this).
var ErrScopedIntegrity = errors.New("scoped secret integrity error")

// ResolveScopedValues returns every scoped secret in stackDir (a
// .sc/stacks/<stack> directory) that ANY of privateKeys is a recipient of. This is
// the deploy-time read hook: the merge is keyed off the AMBIENT/CI KEYS, not a
// config field, so the pull_request clamp is cryptographic — a job holding only
// the "pr" scope key literally cannot decrypt secrets.prod.yaml (it is not a
// recipient), with no config surface to subvert.
//
// privateKeys are candidate PEM keys (the ambient config key plus any CI scope
// keys such as SC_KEY_PR / SC_SCOPE_KEY); an empty or unparseable candidate is
// skipped so callers can pass everything they have.
//
// Semantics:
//   - No usable key, or no secrets.<scope>.yaml in stackDir → empty map, no error
//     (repos that have not adopted scopes are entirely unaffected).
//   - A scope file no candidate key is a recipient of is skipped (least privilege).
//   - A value a key IS a recipient of but cannot decrypt is a HARD error tagged
//     ErrScopedIntegrity (tampered ciphertext / broken binding) — never a silent
//     skip (RFC hard-fail).
//   - A corrupt or renamed scope file, or the same key in two openable scopes, is a
//     HARD error tagged ErrScopedIntegrity.
//
// The caller merges the result into the whole-file store's values WITHOUT
// overwriting existing keys, so a scoped value can never change the meaning of a
// secret already resolved from the legacy store.
func ResolveScopedValues(stackDir string, privateKeys []string) (map[string]string, error) {
	out := map[string]string{}

	entries, err := os.ReadDir(stackDir)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list %s for scoped secrets", stackDir)
	}
	var scopeFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ScopeNameFromFile(e.Name()) == "" {
			continue // not a secrets.<scope>.yaml (legacy secrets.yaml is excluded)
		}
		scopeFiles = append(scopeFiles, filepath.Join(stackDir, e.Name()))
	}
	if len(scopeFiles) == 0 {
		return out, nil // no scopes → do not even parse keys
	}

	// Map every usable candidate key's fingerprint to the key itself. Unparseable
	// or empty candidates are skipped (a caller may pass several).
	fpToKey := map[string]string{}
	for _, pk := range privateKeys {
		if strings.TrimSpace(pk) == "" {
			continue
		}
		fp, _, ferr := privateKeyFingerprint(pk)
		if ferr != nil {
			continue
		}
		fpToKey[fp] = pk
	}
	if len(fpToKey) == 0 {
		return out, nil // no usable key → nothing is openable
	}

	origin := map[string]string{} // key -> scope, to detect cross-scope duplicates
	for _, path := range scopeFiles {
		f, err := LoadScopeFile(path)
		if err != nil {
			return nil, errors.Wrapf(ErrScopedIntegrity, "%v", err)
		}
		// Which candidate key (if any) is a recipient of this scope?
		var openKey string
		for _, r := range f.Recipients {
			rfp, ferr := recipientFingerprint(r)
			if ferr != nil {
				continue
			}
			if k, ok := fpToKey[rfp]; ok {
				openKey = k
				break
			}
		}
		if openKey == "" {
			continue // not our scope
		}
		for _, key := range f.Keys() {
			if prev, dup := origin[key]; dup {
				return nil, errors.Wrapf(ErrScopedIntegrity, "secret %q is present in two openable scopes (%q and %q); resolution is ambiguous — run `sc secrets scope lint`", key, prev, f.Scope)
			}
			val, derr := f.Get(key, openKey)
			if derr != nil {
				return nil, errors.Wrapf(ErrScopedIntegrity, "scope %q: failed to decrypt %q that this key is a recipient of (tampered ciphertext?): %v", f.Scope, key, derr)
			}
			out[key] = val
			origin[key] = f.Scope
		}
	}
	return out, nil
}
