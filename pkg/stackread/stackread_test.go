// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package stackread

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestRegisterSchemaTypes_LandsEveryTypeKind locks in that every type
// field a plugin's SchemaResponse can advertise ends up in
// api.pluginOwnedTypes with the right PluginOwnedKind + plugin name.
// If a new type kind is added to the proto (template_types, …), this
// test flags that stackread's registerSchemaTypes needs a new branch.
func TestRegisterSchemaTypes_LandsEveryTypeKind(t *testing.T) {
	schema := &scpluginv1.SchemaResponse{
		ResourceTypes:         []string{"smoke-bucket", "smoke-lambda"},
		RegistrarTypes:        []string{"smoke-dns"},
		StateStoreTypes:       []string{"smoke-state"},
		SecretsProviderTypes:  []string{"smoke-secrets"},
		ComputeProcessorTypes: []string{"smoke-bucket"},
		VmDrivers: []*scpluginv1.VmDriverSpec{
			{Driver: "smoke-vm"},
			nil, // defensive — a nil entry must not crash
		},
	}
	registerSchemaTypes("smoke-plugin", schema)

	cases := []struct {
		name string
		kind api.PluginOwnedKind
	}{
		{"smoke-bucket", api.PluginOwnedKindComputeProcessor},
		{"smoke-lambda", api.PluginOwnedKindResource},
		{"smoke-dns", api.PluginOwnedKindRegistrar},
		{"smoke-state", api.PluginOwnedKindStateStore},
		{"smoke-secrets", api.PluginOwnedKindSecretsProvider},
		{"smoke-vm", api.PluginOwnedKindVmDriver},
	}
	for _, tc := range cases {
		binding, ok := api.IsPluginOwnedType(tc.name)
		require.True(t, ok, "type %q must be registered", tc.name)
		require.Equal(t, tc.kind, binding.Kind, "type %q kind mismatch", tc.name)
		require.Equal(t, "smoke-plugin", binding.PluginName, "type %q plugin-name mismatch", tc.name)
	}

	// Keep the mapping idempotent — re-registering must not panic or
	// clobber the plugin name.
	registerSchemaTypes("smoke-plugin", schema)
	binding, ok := api.IsPluginOwnedType("smoke-bucket")
	require.True(t, ok)
	require.Equal(t, "smoke-plugin", binding.PluginName)
}

// TestRegisterPluginOwnedType_DoesNotClobberTypedDecoder locks in the
// additive invariant: a type whose typed decoder is already registered
// (via api.RegisterProviderConfig at init() time) is NOT overwritten
// by the opaque decoder when the same plugin advertises it. This keeps
// plugins whose /api module still blank-imports working under the
// pre-parse path.
func TestRegisterPluginOwnedType_DoesNotClobberTypedDecoder(t *testing.T) {
	const typeName = "pre-registered-typed"

	sentinelCalled := false
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		typeName: func(c *api.Config) (api.Config, error) {
			sentinelCalled = true
			return *c, nil
		},
	})

	// Simulate a plugin advertising the same type via Schema.
	api.RegisterPluginOwnedType(typeName, api.PluginOwnedKindResource, "smoke-plugin")

	// Both the binding (plugin ownership for A2 routing) AND the
	// original typed decoder (parse-time validation) must survive.
	binding, ok := api.IsPluginOwnedType(typeName)
	require.True(t, ok)
	require.Equal(t, "smoke-plugin", binding.PluginName)

	// Decoding via DetectTemplateType must still hit the sentinel
	// (NOT the opaque decoder) — i.e. the typed decoder was NOT
	// replaced when RegisterPluginOwnedType ran.
	_, err := api.DetectTemplateType(api.StackDescriptor{Type: typeName, Config: api.Config{}})
	require.NoError(t, err)
	require.True(t, sentinelCalled, "typed decoder was clobbered by the opaque one")
}

// TestCachedSchema_RoundTrip exercises WriteCachedSchema +
// CachedSchema against a tmpdir cache root, verifying the per-version
// cache survives a parser restart.
func TestCachedSchema_RoundTrip(t *testing.T) {
	tmp := t.TempDir()

	spec := api.PluginSpec{
		Source: &api.PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-smoke",
			Version:    "v2026.11.02-abcd1234",
		},
	}
	schema := &scpluginv1.SchemaResponse{
		PluginVersion: "smoke-0.0.1",
		ResourceTypes: []string{"smoke-bucket"},
	}

	// First parse — nothing cached.
	_, hit := scPlugin.CachedSchema(spec, tmp)
	require.False(t, hit, "no cache entry yet, must miss")

	// Write.
	require.NoError(t, scPlugin.WriteCachedSchema(spec, tmp, schema))

	// Cache file lands under <root>/<repo>/<version>/.schema-cache.json.
	expected := filepath.Join(tmp, spec.Source.Repository, spec.Source.Version, ".schema-cache.json")
	_, err := os.Stat(expected)
	require.NoError(t, err, "cache file at %s not written", expected)

	// Second parse — hit returns the exact schema.
	got, hit := scPlugin.CachedSchema(spec, tmp)
	require.True(t, hit, "write-then-read must hit")
	require.Equal(t, "smoke-0.0.1", got.GetPluginVersion())
	require.Equal(t, []string{"smoke-bucket"}, got.GetResourceTypes())
}

// TestCachedSchema_PathSpecSkipsCache locks in that a path:-mode plugin
// spec (dev workflow) bypasses the per-version cache — path: binaries
// are not content-addressed by version, so a cached schema for them
// could be stale across edits.
func TestCachedSchema_PathSpecSkipsCache(t *testing.T) {
	tmp := t.TempDir()
	spec := api.PluginSpec{Path: "/tmp/dev-plugin"}
	schema := &scpluginv1.SchemaResponse{PluginVersion: "dev"}

	require.NoError(t, scPlugin.WriteCachedSchema(spec, tmp, schema))
	// Write is a no-op for path: specs — no cache file is created.
	entries, _ := os.ReadDir(tmp)
	require.Empty(t, entries, "path: specs must not pollute the cache dir")

	_, hit := scPlugin.CachedSchema(spec, tmp)
	require.False(t, hit, "path: specs always miss the cache")
}
