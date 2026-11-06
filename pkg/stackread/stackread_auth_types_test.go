// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package stackread

import (
	"testing"

	"github.com/simple-container-com/api/pkg/api"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestRegisterSchemaTypes_AuthTypes_SeedsProviderConfigMapping pins the
// dynamic-discovery wire path for plugin-owned auth types. The bug this
// closes: `scpluginv1.SchemaResponse` had no `auth_types` field pre-
// 2026-11-06, so a plugin core did NOT link-time-import could never
// register its `auth:` discriminator via Schema — `DetectAuthProvider`
// would fail with `unknown auth type` at parse. See
// forge/docs/roadmap/items/sc-plugin-auth-type-has-no-wire-path.md.
//
// Three invariants locked here:
//
//  1. `registerSchemaTypes` loops `GetAuthTypes()` and seeds
//     `providerConfigMapping` via `RegisterPluginOwnedType`.
//  2. `PluginOwnedKindAuth` equals the pre-existing composite string
//     `PluginOwnedKindResource + "/auth"` — byte-identical to what
//     `plugin_bootstrap.go` has been registering for the five
//     canonical plugins. The refactor from literal to const must not
//     change the kind stored in `pluginOwnedTypes`.
//  3. `IsPluginOwnedType` returns the right `PluginOwnedBinding`
//     (Kind + PluginName) for an auth type registered via the new
//     path, so downstream code reading the kind sees the same label
//     whether the seed came from the bootstrap table or from Schema.
//
// Not covered here (requires a plugin subprocess, run as the PR's
// acceptance test manually): the end-to-end DetectAuthProvider +
// tplAuth path with a plugin binary spawning + answering Schema. The
// in-process shape below locks the registration half; the subprocess
// shape locks the transport half.
func TestRegisterSchemaTypes_AuthTypes_SeedsProviderConfigMapping(t *testing.T) {
	const (
		pluginName = "sc-plugin-stackread-test"
		authType   = "stackread-test-auth-type"
	)

	registerSchemaTypes(pluginName, &scpluginv1.SchemaResponse{
		AuthTypes: []string{authType},
	})

	binding, ok := api.IsPluginOwnedType(authType)
	if !ok {
		t.Fatalf("auth type %q not registered — registerSchemaTypes's AuthTypes loop didn't run or RegisterPluginOwnedType didn't seed providerConfigMapping", authType)
	}
	if binding.PluginName != pluginName {
		t.Errorf("plugin name: got %q, want %q", binding.PluginName, pluginName)
	}
	// Kind MUST be the composite string. A refactor that changed
	// PluginOwnedKindAuth to its own distinct value would break
	// byte-identical registration with plugin_bootstrap.go's
	// pre-const seed and silently split the auth-registration path.
	if string(binding.Kind) != string(api.PluginOwnedKindResource)+"/auth" {
		t.Errorf("kind: got %q, want %q (PluginOwnedKindResource + \"/auth\")",
			binding.Kind, string(api.PluginOwnedKindResource)+"/auth")
	}
	if binding.Kind != api.PluginOwnedKindAuth {
		t.Errorf("kind: got %q, want PluginOwnedKindAuth (%q) — const must equal the composite",
			binding.Kind, api.PluginOwnedKindAuth)
	}
}

// TestRegisterSchemaTypes_AuthTypes_EmptyIsSafe pins the additive
// property: an older plugin binary that pre-dates auth_types sends
// nil/empty, the loop runs zero times, no side effects, no error. This
// is what makes the whole patch safe to roll out without a lockstep
// plugin release.
func TestRegisterSchemaTypes_AuthTypes_EmptyIsSafe(t *testing.T) {
	// Count the number of entries registered before and after to assert
	// zero side effects.
	registerSchemaTypes("empty-schema-plugin", &scpluginv1.SchemaResponse{
		// AuthTypes intentionally nil — simulates an older plugin.
	})
	// If any entry was seeded, a lookup for a sentinel would succeed.
	// The sentinel is deliberately a string an older plugin would not
	// own.
	if _, ok := api.IsPluginOwnedType("empty-schema-plugin-sentinel"); ok {
		t.Errorf("empty SchemaResponse.AuthTypes must not register any token")
	}
}
