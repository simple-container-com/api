// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// Every api.Provisioner entry point that reaches the Pulumi backend has to run
// the pre-engine plugin load first, because when the state storage is
// plugin-owned the sidecar is the only thing that knows the backend URL. An
// `s3-bucket` state-storage declares `bucketName:` and nothing else; the plugin
// assembles `s3://<bucket>` in InitStateStore and publishes it via
// PULUMI_BACKEND_URL, which login() reads only if the sidecar already ran.
//
// Skip the load and StorageUrl() returns the empty string that
// *api.PluginFieldConfig yields for a key the operator never wrote, so
// diy.Login fails with
//
//	diy URL  has an illegal prefix; expected one of: azblob, file, gs, postgres, s3
//
// — an error naming no stack, no plugin and no field, with a telltale double
// space where the URL should be. OutputsStack and CancelStack were both missing
// the load; `sc stack outputs -s infra` against forge's s3-bucket backend is
// what surfaced it.
//
// The table is the guard: a new entry point added without the load fails here
// rather than at an operator's terminal. Each case drives the real method with
// a plugin spec that cannot resolve, so the only way to pass is to attempt the
// load — a stubbed-out bundle would make the test vacuous.
func TestBackendEntryPointsLoadPreEnginePlugins(t *testing.T) {
	RegisterTestingT(t)

	// A path that cannot be spawned. The load fails at the plugin client, far
	// earlier than any backend login, so no case here touches the network or a
	// real checkpoint.
	stackWithUnresolvablePlugin := func(t *testing.T) api.Stack {
		return api.Stack{
			Name: "infra",
			Server: api.ServerDescriptor{
				Plugins: map[string]api.PluginSpec{
					"nonexistent": {Path: filepath.Join(t.TempDir(), "sc-plugin-nonexistent")},
				},
			},
		}
	}

	cases := []struct {
		name string
		call func(context.Context, *pulumi, *api.ConfigFile, api.Stack) error
	}{
		{
			// The two this test was written for.
			name: "OutputsStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				_, err := p.OutputsStack(ctx, cfg, s, api.StackParams{StackName: "infra"})
				return err
			},
		},
		{
			name: "CancelStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				return p.CancelStack(ctx, cfg, s, api.StackParams{StackName: "infra"})
			},
		},
		// The six that already had it, pinned so a refactor cannot quietly drop
		// one and reintroduce the same class of bug elsewhere.
		{
			name: "ProvisionStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				return p.ProvisionStack(ctx, cfg, s, api.ProvisionParams{})
			},
		},
		{
			name: "DeployStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				return p.DeployStack(ctx, cfg, s, api.DeployParams{})
			},
		},
		{
			name: "PreviewStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				_, err := p.PreviewStack(ctx, cfg, s, api.ProvisionParams{})
				return err
			},
		},
		{
			name: "PreviewChildStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				_, err := p.PreviewChildStack(ctx, cfg, s, api.DeployParams{})
				return err
			},
		},
		{
			name: "DestroyParentStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				return p.DestroyParentStack(ctx, cfg, s, api.DestroyParams{}, true)
			},
		},
		{
			name: "DestroyChildStack",
			call: func(ctx context.Context, p *pulumi, cfg *api.ConfigFile, s api.Stack) error {
				return p.DestroyChildStack(ctx, cfg, s, api.DestroyParams{}, true)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)

			p := &pulumi{logger: logger.New()}
			err := tc.call(context.Background(), p, &api.ConfigFile{}, stackWithUnresolvablePlugin(t))

			Expect(err).To(HaveOccurred(),
				"%s accepted a stack whose declared plugin cannot be spawned", tc.name)
			// The wrap is the evidence the load was attempted. Without it the
			// call proceeds to the backend and fails with diy's
			// `illegal prefix` instead — or, worse, succeeds against a stale
			// PULUMI_BACKEND_URL left in the environment.
			Expect(err.Error()).To(ContainSubstring("pre-engine plugin load"),
				"%s failed for some other reason than the pre-engine plugin load: %v", tc.name, err)
		})
	}

	// The table above is hand-written, so on its own it would let entry point
	// #9 through the same door: a new api.Provisioner method just wouldn't be
	// listed, and nothing would complain. Derive the expected set from the
	// interface instead and fail closed.
	//
	// The rule is structural, not an allowlist — a method qualifies when it
	// takes a context AND an api.Stack, which is exactly the shape of a call
	// that resolves a stack against the backend. SetPublicKey and
	// SetConfigReader are plain setters and match neither, so they drop out on
	// their own rather than needing to be named here (and a future setter will
	// too).
	covered := map[string]bool{}
	for _, tc := range cases {
		covered[tc.name] = true
	}
	iface := reflect.TypeOf((*api.Provisioner)(nil)).Elem()
	for i := range iface.NumMethod() {
		m := iface.Method(i)
		if !takesContextAndStack(m.Type) {
			continue
		}
		Expect(covered).To(HaveKey(m.Name),
			"api.Provisioner.%s resolves a stack against the backend but is not in this test's "+
				"table. Add it — and make sure it calls loadPreEnginePlugins, or a plugin-owned "+
				"state storage will fail it with `diy URL  has an illegal prefix`.", m.Name)
	}
}

// takesContextAndStack reports whether an api.Provisioner method's signature
// is the backend-touching shape: a context plus the stack to resolve. Method
// values off an interface type have no receiver in their In() list.
func takesContextAndStack(fn reflect.Type) bool {
	if fn.NumIn() == 0 || fn.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return false
	}
	stackType := reflect.TypeOf(api.Stack{})
	for i := 1; i < fn.NumIn(); i++ {
		if fn.In(i) == stackType {
			return true
		}
	}
	return false
}

// A stack that declares no plugins must not be charged for the lookup: every
// in-tree state storage (fs, and the pulumi-cloud backend) still has to work
// with no `plugins:` block at all, and the entry points above call this
// unconditionally.
func TestLoadPreEnginePluginsNoOpsWithoutPlugins(t *testing.T) {
	RegisterTestingT(t)

	p := &pulumi{logger: logger.New()}
	Expect(p.loadPreEnginePlugins(context.Background(), api.Stack{Name: "infra"})).To(Succeed())
	Expect(p.preEnginePlugins).To(BeNil(),
		"a bundle was created for a stack with no plugins; closePreEnginePlugins would then tear down nothing")
}

// TestLoadForBackend_CacheSkipsPluginWithoutBackendTypes — the whole point of
// the cache-aware variant: a plugin whose cached schema advertises NEITHER
// state-store types NOR secrets-provider types doesn't contribute to
// `sc stack outputs` / `sc cancel` opening the backend, so we skip the spawn.
//
// Fixture: seed the per-version cache with a schema that lists only
// resource/compute-processor types; call LoadForBackend; verify the bundle
// stays empty (no client was spawned, no sidecar registered). Correctness
// invariant on top: a plugin whose cached schema DOES advertise state-store
// or secrets-provider types IS spawned on the same code path — covered by
// the fallback case in TestBackendEntryPointsLoadPreEnginePlugins above
// (that uses a path: spec, which bypasses the cache entirely and takes the
// full spawn path — proving the fallback works for any cache-miss shape,
// including the "cache absent" shape the first-ever run hits).
func TestLoadForBackend_CacheSkipsPluginWithoutBackendTypes(t *testing.T) {
	RegisterTestingT(t)

	tmp := t.TempDir()

	// Seed the cache for a plugin spec with a schema that advertises only
	// a resource type — no state-store, no secrets-provider.
	spec := api.PluginSpec{Source: &api.PluginSource{
		Type:       "github-release",
		Repository: "example/resource-only-plugin",
		Version:    "v1.0.0",
	}}
	schema := &scpluginv1.SchemaResponse{
		PluginVersion: "1.0.0",
		ResourceTypes: []string{"example-widget"},
	}
	Expect(scPlugin.WriteCachedSchema(spec, tmp, schema)).To(Succeed())

	// loadForBackendWithCache is LoadForBackend's testable seam — same path,
	// cacheRoot injected. The schema has no backend-contributing types → skip
	// entirely. No spawn attempt is made (crucially — the plugin binary does
	// not exist at the resolved path).
	b := newPreEnginePluginBundle(context.Background(), logger.New())
	defer b.Close()
	Expect(b.loadForBackendWithCache(context.Background(), map[string]api.PluginSpec{
		"resource-only": spec,
	}, tmp)).To(Succeed())
	Expect(b.clients).To(BeEmpty(), "a plugin without state-store or secrets-provider must not be spawned on the backend-only load")
	Expect(b.schemas).To(BeEmpty(), "schemas map must stay empty too — no sidecar registration happened")
}

// TestLoadForBackend_CacheMissFallsBackToFullSpawn — the correctness
// guarantee. If the cache has no entry (fresh install / path: spec / corrupt
// cache), we fall through to full spawn. A nonexistent plugin binary makes
// that spawn fail, which is the only way to prove this test's assertion
// (a stub would make the test vacuous).
func TestLoadForBackend_CacheMissFallsBackToFullSpawn(t *testing.T) {
	RegisterTestingT(t)

	// A path: spec — schemaCachePath returns (_, false), so CachedSchema
	// always misses, triggering the full-spawn fallback.
	spec := api.PluginSpec{Path: filepath.Join(t.TempDir(), "sc-plugin-missing")}

	b := newPreEnginePluginBundle(context.Background(), logger.New())
	defer b.Close()
	err := b.LoadForBackend(context.Background(), map[string]api.PluginSpec{
		"missing": spec,
	})
	Expect(err).To(HaveOccurred(), "cache miss must fall back to full spawn, which fails here because the binary does not exist")
}
