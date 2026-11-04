// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package stackread

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
)

// TestReadServerDescriptor_RecordsAutoInjectedPluginSpec is the
// regression anchor for the alphamind-blog cloud-compose deploy failure
// (GitHub Actions run 41823803096):
//
//	stackread: no plugin spec recorded for plugin "aws" — did the pre-parse run?
//
// The parent server.yaml had an `ecs-fargate` template and NO `plugins:`
// block. DiscoverPluginTypes ran on the raw descriptor, saw no plugins
// and recorded nothing; api.ValidatePluginRequirements then
// auto-injected `plugins.aws` during api.ReadServerConfigs, but that
// spec never reached the ConvertStackConfig dialler. The read must
// succeed AND the auto-injected aws spec must be recorded.
func TestReadServerDescriptor_RecordsAutoInjectedPluginSpec(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const injectedVersion = "v9.9.9-autoinject-regr"
	var resolvedRepos []string
	restore := api.SetLatestVersionResolver(func(repo string) (string, error) {
		resolvedRepos = append(resolvedRepos, repo)
		return injectedVersion, nil
	})
	defer restore()

	// Isolate the package-level registry from other tests.
	prevSpecs := pluginSpecsByName
	pluginSpecsByName = map[string]api.PluginSpec{}
	defer func() { pluginSpecsByName = prevSpecs }()

	dir := t.TempDir()
	serverYaml := filepath.Join(dir, api.ServerDescriptorFileName)
	require.NoError(t, os.WriteFile(serverYaml, []byte(`schemaVersion: 1.0
provisioner:
  inherit: common
templates:
  stack-per-app:
    type: ecs-fargate
    config:
      credentials: "${auth:aws}"
      account: "${auth:aws.projectId}"
`), 0o644))

	desc, err := ReadServerDescriptor(context.Background(), serverYaml)
	require.NoError(t, err, "a server.yaml with an ecs-fargate template and no plugins: block must read")

	require.Contains(t, desc.Plugins, "aws", "ValidatePluginRequirements must auto-inject plugins.aws")
	require.Equal(t, []string{"simple-container-com/sc-plugin-aws"}, resolvedRepos)

	spec, ok := pluginSpecsByName["aws"]
	require.True(t, ok, "the auto-injected aws spec must be recorded for the ConvertStackConfig dialler")
	require.NotNil(t, spec.Source)
	require.Equal(t, "simple-container-com/sc-plugin-aws", spec.Source.Repository)
	require.Equal(t, injectedVersion, spec.Source.Version)
}
