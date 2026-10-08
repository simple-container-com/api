// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
	"sort"
	"strings"
)

// EnvScopeKeys returns the scope keys a CI job supplies without a full config: the
// generic SC_SCOPE_KEY and every SC_KEY_<SCOPE> whose <SCOPE> maps back to a valid
// scope name (uppercase, '-' as '_'), so an unrelated SC_KEY_* variable is not tried
// as a decryption key. A job with several scope keys resolves the union of its scopes.
func EnvScopeKeys() []string {
	var keys []string
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" {
		keys = append(keys, v)
	}
	for _, e := range os.Environ() {
		name, val, ok := strings.Cut(e, "=")
		if !ok || strings.TrimSpace(val) == "" || !strings.HasPrefix(name, "SC_KEY_") {
			continue
		}
		scopeName := strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, "SC_KEY_"), "_", "-"))
		if ValidateScopeName(scopeName) != nil {
			continue
		}
		keys = append(keys, val)
	}
	return keys
}

// InvalidEnvScopeKeys names the SC_SCOPE_KEY and SC_KEY_<SCOPE> variables that
// are set but do not parse as a private key. EnvScopeKeys still returns their
// values, and an Opener skips them; a caller reports them so a broken CI secret is
// not read as "not a recipient".
func InvalidEnvScopeKeys() []string {
	var bad []string
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" && ValidatePrivateKey(v) != nil {
		bad = append(bad, "SC_SCOPE_KEY")
	}
	for _, e := range os.Environ() {
		name, val, ok := strings.Cut(e, "=")
		if !ok || strings.TrimSpace(val) == "" || !strings.HasPrefix(name, "SC_KEY_") {
			continue
		}
		if ValidateScopeName(strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, "SC_KEY_"), "_", "-"))) != nil {
			continue
		}
		if ValidatePrivateKey(val) != nil {
			bad = append(bad, name)
		}
	}
	sort.Strings(bad)
	return bad
}
