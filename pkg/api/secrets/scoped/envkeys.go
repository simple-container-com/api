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

// InvalidEnvScopeKeys names the SC_SCOPE_KEY variable and the SC_KEY_<SCOPE>
// variables of the given scopes that are set but do not parse as a private key.
// Only scopes the caller knows exist are checked: SC_KEY_FILE or SC_KEY_ID may
// be some other tool's variable, not a key to a scope named "file". EnvScopeKeys
// still returns their values, and an Opener skips them; a caller reports them so
// a broken CI secret is not read as "not a recipient".
func InvalidEnvScopeKeys(scopes []string) []string {
	var bad []string
	if v := os.Getenv("SC_SCOPE_KEY"); strings.TrimSpace(v) != "" && ValidatePrivateKey(v) != nil {
		bad = append(bad, "SC_SCOPE_KEY")
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		name := ScopeKeyEnvName(scope)
		if seen[name] {
			continue
		}
		seen[name] = true
		if v := os.Getenv(name); strings.TrimSpace(v) != "" && ValidatePrivateKey(v) != nil {
			bad = append(bad, name)
		}
	}
	sort.Strings(bad)
	return bad
}

// ScopeKeyEnvName is the environment variable that holds a scope's CI key:
// SC_KEY_<SCOPE>, uppercase, with '-' as '_'.
func ScopeKeyEnvName(scope string) string {
	return "SC_KEY_" + strings.ToUpper(strings.ReplaceAll(scope, "-", "_"))
}
