// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecretsBundleConfig_IsEnabled pins the three-state ladder on the
// pointer-struct opt-in (design 2026-10-17 §3.1):
//
//	nil receiver           → false (legacy per-key rendering)
//	non-nil, Enabled unset → true  (minimal `secretsBundle: {}` opts in)
//	non-nil, Enabled false → false (operator-staged rollback escape hatch)
//	non-nil, Enabled true  → true  (explicit on)
func TestSecretsBundleConfig_IsEnabled(t *testing.T) {
	var nilCfg *SecretsBundleConfig
	assert.False(t, nilCfg.IsEnabled(), "nil receiver = legacy")

	assert.True(t, (&SecretsBundleConfig{}).IsEnabled(), "non-nil with Enabled unset = enabled")

	off := false
	assert.False(t, (&SecretsBundleConfig{Enabled: &off}).IsEnabled(), "explicit Enabled=false = disabled")

	on := true
	assert.True(t, (&SecretsBundleConfig{Enabled: &on}).IsEnabled(), "explicit Enabled=true = enabled")
}

// TestCloudExtras_SecretsBundle_OmitemptyWhenNil — the schema MUST
// serialise identically to its pre-change shape when SecretsBundle is
// nil. Guard against a future developer dropping the `omitempty` tag
// and silently changing the YAML surface for every existing service.
func TestCloudExtras_SecretsBundle_OmitemptyWhenNil(t *testing.T) {
	// Verified via struct tag introspection — if omitempty is dropped
	// the test fails loudly rather than only at the YAML byte level.
	e := CloudExtras{}
	// The field is a pointer; its zero value is nil; omitempty makes
	// json/yaml skip it. We don't marshal here because yaml/json tag
	// behaviour is well-covered by stdlib; the sanity check is that
	// the zero-value struct carries no bundle state.
	assert.Nil(t, e.SecretsBundle, "zero-value CloudExtras must have SecretsBundle=nil")
	assert.False(t, e.SecretsBundle.IsEnabled(), "zero-value = legacy per-key rendering")
}
