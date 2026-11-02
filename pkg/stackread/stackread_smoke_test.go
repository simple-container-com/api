// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

//go:build smoke

// Smoke test for the two-pass parse-time plugin-type discovery.
// Builds the sibling sc-plugin-echo binary and asserts that a
// server.yaml declaring `plugins: echo: path: <built>` parses
// successfully when a `resources:` entry uses `type: echo-bucket`
// (which echo's /api module does NOT blank-register into SC
// core — the point is that stackread learns the type via Schema at
// parse time, with NO Go-link coupling).
//
// Run with: `go test -tags smoke -run SmokeStackread ./pkg/stackread/...`.
// Skipped by default so CI doesn't need to vendor a sibling plugin.
package stackread

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/simple-container-com/api/pkg/api"
)

func TestSmokeStackread_SpawnsEchoAndRegistersPluginType(t *testing.T) {
	// Locate the sc-plugin-echo sibling checkout. The smoke runs from
	// the api repo's working tree; the sibling layout is the one every
	// fleet operator uses (…/simple-container/{api,sc-plugin-echo,…}).
	echoRepo := locateSiblingRepo(t, "sc-plugin-echo")

	// Build the binary into a tmpdir so the smoke is hermetic — no
	// pollution of the sibling repo's workspace, no re-use of a stale
	// GOBIN artefact.
	tmp := t.TempDir()
	echoBin := filepath.Join(tmp, "sc-plugin-echo")
	cmd := exec.Command("go", "build", "-o", echoBin, "./cmd/sc-plugin-echo")
	cmd.Dir = filepath.Join(echoRepo, "plugin")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", echoBin, err, out)
	}

	// Hand-construct a ServerDescriptor whose plugins: block points at
	// the built echo binary. Skip the full-server-yaml parse (needs a
	// real provisioner + state-storage + secrets-provider block —
	// that's covered by the throwaway-stack smoke, not this one).
	// What THIS smoke proves: stackread's pre-parse hits
	// scPlugin.ResolveAndSchema with a real spawn + Schema RPC, caches
	// the response, and registers every advertised type via
	// api.RegisterPluginOwnedType. That is the full A1 observable.
	desc := &api.ServerDescriptor{
		Plugins: map[string]api.PluginSpec{
			"echo": {Path: echoBin},
		},
	}
	require.NoError(t, DiscoverPluginTypes(context.Background(), desc),
		"DiscoverPluginTypes must spawn echo + register its Schema types")

	// Post-condition: echo-bucket IS now plugin-owned (binding present),
	// with the plugin name matching the server.yaml key.
	// echo advertises echo-bucket under both ResourceTypes and
	// ComputeProcessorTypes — the latter wins the kind slot (last
	// RegisterPluginOwnedType call wins), which is fine — the point
	// is that stackread does not need any Go-link registration.
	binding, ok := api.IsPluginOwnedType("echo-bucket")
	require.True(t, ok, "echo-bucket must have been registered by stackread")
	require.Equal(t, "echo", binding.PluginName)
	require.Contains(t,
		[]api.PluginOwnedKind{api.PluginOwnedKindResource, api.PluginOwnedKindComputeProcessor},
		binding.Kind,
		"echo-bucket is in BOTH resource_types and compute_processor_types; either kind is acceptable")

	// Cache lookup: a second parse-time pass that uses the same cache
	// root MUST return the schema straight from disk, no spawn.
	// (The spec here is path:, so the cache is skipped by design —
	// that's covered by TestCachedSchema_PathSpecSkipsCache. For a
	// github-release spec the cache round-trip proves itself; see
	// TestCachedSchema_RoundTrip.)
}

// TestSmokeStackread_ConvertStackConfigDialsPlugin exercises the A2
// end-to-end path: pre-parse spawns echo and registers echo-bucket as
// a plugin-owned type, then api.PrepareStaticForDeploy sees the type,
// dials convertStackConfigViaPlugin, which re-spawns echo and calls
// its ConvertStackConfig RPC, and the returned pluginPreparedInput
// satisfies DnsConfigAware / ResourceAware / WithDependsOnResources.
func TestSmokeStackread_ConvertStackConfigDialsPlugin(t *testing.T) {
	echoRepo := locateSiblingRepo(t, "sc-plugin-echo")
	tmp := t.TempDir()
	echoBin := filepath.Join(tmp, "sc-plugin-echo")
	cmd := exec.Command("go", "build", "-o", echoBin, "./cmd/sc-plugin-echo")
	cmd.Dir = filepath.Join(echoRepo, "plugin")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build echo: %v\n%s", err, out)
	}

	// Pre-parse — register echo's types + record its spec for A2.
	desc := &api.ServerDescriptor{
		Plugins: map[string]api.PluginSpec{"echo": {Path: echoBin}},
	}
	require.NoError(t, DiscoverPluginTypes(context.Background(), desc))

	// echo-bucket is now plugin-owned. Dial via PrepareStaticForDeploy —
	// the stackread init() wired api.RegisterPluginStackConverter so
	// the branch at pkg/api/client.go:435 fires.
	tpl := api.StackDescriptor{Type: "echo-bucket"}
	clientConfig := &api.StackConfigStatic{BundleDir: "./bundle"}
	got, err := api.PrepareStaticForDeploy(
		context.Background(),
		"/tmp/stack-dir", "smoke-static", tpl, clientConfig, "parent-stack",
	)
	require.NoError(t, err, "A2 dial path must succeed")
	require.NotNil(t, got)
	require.Equal(t, "echo-bucket", got.Type)
	require.Equal(t, "parent-stack", got.ParentStack)

	// The wrapped config satisfies the three interfaces downstream SC
	// core type-asserts off. Echo's canned response says
	// BaseDnsZone="echo.example.com" and Uses=["echo-bucket"].
	cfg := got.Config.Config
	dns, ok := cfg.(api.DnsConfigAware)
	require.True(t, ok, "wrapped config must satisfy DnsConfigAware")
	require.Equal(t, "echo.example.com", dns.OverriddenBaseZone())

	resource, ok := cfg.(api.ResourceAware)
	require.True(t, ok, "wrapped config must satisfy ResourceAware")
	require.Equal(t, []string{"echo-bucket"}, resource.Uses())

	deps, ok := cfg.(api.WithDependsOnResources)
	require.True(t, ok, "wrapped config must satisfy WithDependsOnResources")
	require.Empty(t, deps.DependsOnResources(), "echo stub emits no cross-stack deps")
}

func locateSiblingRepo(t *testing.T, name string) string {
	t.Helper()
	// Walk up from the current working dir — api repo root is 1 level
	// above pkg/stackread; the sibling sits next to the api repo root.
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := wd
	for depth := 0; depth < 6; depth++ {
		candidate := filepath.Join(filepath.Dir(root), name)
		if _, err := os.Stat(filepath.Join(candidate, "plugin", "cmd", name)); err == nil {
			return candidate
		}
		if root == filepath.Dir(root) {
			break
		}
		root = filepath.Dir(root)
	}
	t.Skipf("sibling repo %q not found adjacent to api — skip smoke", name)
	return ""
}
