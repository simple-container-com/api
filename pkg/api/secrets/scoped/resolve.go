// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// ResolveScopedValues returns every scoped secret in stackDir (a
// .sc/stacks/<stack> directory) that privateKey is a recipient of. This is the
// deploy-time read hook: the merge is keyed off the AMBIENT KEY, not a config
// field, so the pull_request clamp is cryptographic — a job holding only the "pr"
// scope key literally cannot decrypt secrets.prod.yaml (it is not a recipient),
// with no config surface to subvert.
//
// Semantics:
//   - No private key, or no secrets.<scope>.yaml in stackDir → empty map, no error
//     (repos that have not adopted scopes are entirely unaffected).
//   - A scope file the key is NOT a recipient of is skipped (least privilege — you
//     simply do not see other scopes' values).
//   - A value the key IS a recipient of but cannot decrypt is a HARD error
//     (tampered ciphertext / broken binding), never a silent skip (RFC hard-fail).
//   - A corrupt or renamed scope file is a hard error (LoadScopeFile enforces the
//     filename↔scope binding).
//   - The same key appearing in two scopes the caller can open is a hard error
//     (ambiguous resolution); `sc secrets scope lint` is expected to prevent it.
//
// The caller merges the result into the whole-file store's values WITHOUT
// overwriting existing keys, so a scoped value can never change the meaning of a
// secret already resolved from the legacy store.
func ResolveScopedValues(stackDir, privateKey string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(privateKey) == "" {
		return out, nil
	}
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
		return out, nil // no scopes → do not even parse the key
	}
	fp, _, err := privateKeyFingerprint(privateKey)
	if err != nil {
		return nil, errors.Wrap(err, "cannot use ambient key to resolve scoped secrets")
	}
	origin := map[string]string{} // key -> scope, to detect cross-scope duplicates
	for _, path := range scopeFiles {
		f, err := LoadScopeFile(path)
		if err != nil {
			return nil, err
		}
		isRecipient := false
		for _, r := range f.Recipients {
			if rfp, ferr := recipientFingerprint(r); ferr == nil && rfp == fp {
				isRecipient = true
				break
			}
		}
		if !isRecipient {
			continue
		}
		for _, key := range f.Keys() {
			if prev, dup := origin[key]; dup {
				return nil, errors.Errorf("secret %q is present in two openable scopes (%q and %q); resolution is ambiguous — run `sc secrets scope lint`", key, prev, f.Scope)
			}
			val, derr := f.Get(key, privateKey)
			if derr != nil {
				return nil, errors.Wrapf(derr, "scope %q: failed to decrypt %q that this key is a recipient of (tampered ciphertext?)", f.Scope, key)
			}
			out[key] = val
			origin[key] = f.Scope
		}
	}
	return out, nil
}
