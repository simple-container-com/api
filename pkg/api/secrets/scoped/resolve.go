// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// ErrScopedIntegrity tags a scoped-resolution failure that must NEVER be treated
// as "secrets simply absent": a value the key is a recipient of but cannot decrypt
// (tamper), a corrupt or renamed scope file, or the same key present in two
// openable scopes (ambiguous). Callers that otherwise tolerate a missing legacy
// secrets.yaml (IgnoreSecretsMissing) MUST still fail on errors.Is(err, this).
var ErrScopedIntegrity = errors.New("scoped secret integrity error")

// ErrScopedUnavailable tags a scoped-resolution failure caused by a transient
// backend fault the caller IS entitled to but could not complete right now — e.g. a
// KMS throttle/outage after SDK retries. It is fatal (a deploy must not proceed with
// a secret it could not resolve) but distinct from ErrScopedIntegrity: it means
// "retry", not "tamper". Like ErrScopedIntegrity it must survive IgnoreSecretsMissing
// (see ReadStacks) — a transient outage is never "secrets simply absent".
var ErrScopedUnavailable = errors.New("scoped secret temporarily unavailable")

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

	// The opener holds every usable candidate SSH key and permits KMS Decrypt via
	// the ambient AWS credentials. KMS is only ever attempted for a value that has a
	// KMS wrap slot AND that no held SSH key opened, so an SSH-only store never calls
	// AWS. Unparseable/empty candidate keys are skipped (a caller may pass several).
	op := NewOpener(privateKeys, true)
	if !op.hasMaterial() {
		return out, nil // nothing usable to open with
	}

	origin := map[string]string{} // key -> scope, to detect cross-scope duplicates
	for _, path := range scopeFiles {
		f, err := LoadScopeFile(path)
		if err != nil {
			return nil, errors.Wrapf(ErrScopedIntegrity, "%v", err)
		}
		keys := f.Keys()
		// All values in a scope file share the same recipient set, so ownership is
		// file-uniform. We are a recipient of this file if EITHER a held SSH key is a
		// declared recipient (offline proof) OR some value opens (establishes KMS
		// recipiency, which cannot be proven offline). Once we know we are a recipient,
		// ANY value that fails to open is tampering (a stripped wrap) — including the
		// FIRST value, so first-value tamper is never swallowed as "not my scope".
		sshDeclared := op.IsDeclaredSSHRecipient(f.Recipients)
		fileOwned := false
		vals := make(map[string]string, len(keys))
		for _, key := range keys {
			val, owned, oerr := f.Open(key, op)
			if oerr != nil {
				if owned {
					// We ARE a recipient of this value but it failed to open: tampered
					// ciphertext, a broken (stack,scope,key) binding, or a wrong-key wrap.
					return nil, errors.Wrapf(ErrScopedIntegrity, "scope %q: failed to open %q that this key/role is a recipient of (tampered ciphertext or broken binding?): %v", f.Scope, key, oerr)
				}
				// A transient/unavailable backend fault (e.g. a KMS throttle/outage) —
				// fatal, but a retry, not tamper. Position-independent: it never masquerades
				// as a stripped wrap regardless of which value hit it.
				return nil, errors.Wrapf(ErrScopedUnavailable, "scope %q: could not open %q: %v", f.Scope, key, oerr)
			}
			if !owned {
				if fileOwned || sshDeclared {
					// We are provably a recipient (an earlier value opened, or a held SSH key
					// is in the declared recipient set) yet this value has no wrap for us —
					// its slot was stripped. Tamper, not least-privilege.
					return nil, errors.Wrapf(ErrScopedIntegrity, "scope %q: value %q is missing this recipient's wrap (tampered?)", f.Scope, key)
				}
				break // not our scope
			}
			fileOwned = true
			vals[key] = val
		}
		if !fileOwned {
			continue
		}
		for _, key := range keys {
			val, ok := vals[key]
			if !ok {
				continue
			}
			if prev, dup := origin[key]; dup {
				return nil, errors.Wrapf(ErrScopedIntegrity, "secret %q is present in two openable scopes (%q and %q); resolution is ambiguous — run `sc secrets scope lint`", key, prev, f.Scope)
			}
			out[key] = val
			origin[key] = f.Scope
		}
	}
	return out, nil
}
