// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package stackread

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// TestDiscoverPluginTypes_StateStoreAndSecretsProviderParse is the
// end-to-end regression anchor for the forge-conductor isolated verify
// failure (run 41670670084, 2026-11-03) against api v2026.11.10:
//
//	failed to read state storage config:
//	unknown provisioner field config type "s3-bucket"
//
// Pre-A3, each plugin's /api/init() populated two maps for every
// state-storage / secrets-provider type: providerConfigMapping (read by
// resource/registrar parse paths) and provisionerFieldConfigMapping
// (read by ReadProvisionerFieldConfig for `provisioner.state-storage.type`
// + `provisioner.secrets-provider.type`). A3 (api #539) stripped the
// init imports; dynamic discovery via stackread.DiscoverPluginTypes +
// api.RegisterPluginOwnedType originally seeded only the first map.
//
// This test exercises the FULL stackread entry point (schema cache ->
// DiscoverPluginTypes -> api.RegisterPluginOwnedType -> both maps),
// then calls api.ReadProvisionerFieldConfig for both arms — the parse
// paths that forge-conductor hit in production. If dynamic discovery
// seeds only providerConfigMapping again, this test fails before
// release instead of after.
func TestDiscoverPluginTypes_StateStoreAndSecretsProviderParse(t *testing.T) {
	// Point the plugin cache at a tmpdir so this unit test is hermetic
	// (scPlugin.DefaultCacheRoot expands to $HOME/.sc/plugins).
	t.Setenv("HOME", t.TempDir())

	const (
		stateStoreToken      = "stackread-regr-s3-bucket"
		secretsProviderToken = "stackread-regr-aws-kms"
	)

	spec := api.PluginSpec{
		Source: &api.PluginSource{
			Type:       "github-release",
			Repository: "simple-container-com/sc-plugin-stackread-regr",
			Version:    "v0.0.1-regr",
		},
	}
	schema := &scpluginv1.SchemaResponse{
		PluginVersion:        "stackread-regr-0.0.1",
		StateStoreTypes:      []string{stateStoreToken},
		SecretsProviderTypes: []string{secretsProviderToken},
	}

	// Pre-seed the schema cache so loadSchema takes the pure-cache
	// branch and never tries to download / spawn a binary.
	require.NoError(t, scPlugin.WriteCachedSchema(spec, scPlugin.DefaultCacheRoot, schema))

	desc := &api.ServerDescriptor{
		Plugins: map[string]api.PluginSpec{"stackread-regr": spec},
	}
	require.NoError(t, DiscoverPluginTypes(context.Background(), desc),
		"DiscoverPluginTypes must succeed off the pre-seeded cache")

	// StateStore arm — this is the exact path forge-conductor failed on.
	gotState, err := api.ReadProvisionerFieldConfig(stateStoreToken, &api.Config{
		Config: map[string]any{"url": "s3://regr-bucket/state"},
	})
	require.NoError(t, err,
		"ReadProvisionerFieldConfig for a plugin-advertised state-storage type must succeed. "+
			"If this fails with 'unknown provisioner field config type', the fix at "+
			"api v2026.11.11 (RegisterPluginOwnedType also seeds provisionerFieldConfigMapping "+
			"for StateStore / SecretsProvider kinds) has regressed.")
	wrappedState, ok := gotState.Config.(*api.PluginFieldConfig)
	require.True(t, ok, "state-storage config must be wrapped in *PluginFieldConfig, got %T", gotState.Config)
	require.Equal(t, stateStoreToken, wrappedState.TypeName)
	require.Equal(t, "s3://regr-bucket/state", wrappedState.URL)

	// SecretsProvider arm — same bug surface, different field.
	gotSecrets, err := api.ReadProvisionerFieldConfig(secretsProviderToken, &api.Config{
		Config: map[string]any{"url": "awskms://alias/regr"},
	})
	require.NoError(t, err,
		"ReadProvisionerFieldConfig for a plugin-advertised secrets-provider type must succeed")
	wrappedSecrets, ok := gotSecrets.Config.(*api.PluginFieldConfig)
	require.True(t, ok)
	require.Equal(t, secretsProviderToken, wrappedSecrets.TypeName)
	require.Equal(t, "awskms://alias/regr", wrappedSecrets.URL)
}
