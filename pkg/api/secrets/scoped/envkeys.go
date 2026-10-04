// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"os"
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
