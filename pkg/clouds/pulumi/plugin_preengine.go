// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"os"
	"sync"

	hclog "github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// preEnginePluginBundle holds plugin subprocesses + their schemas loaded
// BEFORE the Pulumi engine starts. Pre-engine phase of the two-phase plugin
// lifecycle — see forge/docs/design/2026-10-24/sc-plugin-architecture/
// 04-no-destroy-invariant.md Addendum C.
//
// What runs here:
//   - Spawn each plugin binary via plugin.NewClient.
//   - Call Schema to see what state_store_types / secrets_provider_types /
//     resource_types the plugin owns.
//   - Register sidecar InitStateStore handlers in pApi.InitStateStoreFuncByType
//     so SC core's login flow (pkg/clouds/pulumi/login.go) picks them up
//     transparently — same path in-tree types like fs/s3 use.
//
// What does NOT run here: MonitorProxy, resource dispatch. Those are the
// in-engine Phase 2, handled by pluginSession inside the inline RunFunc.
//
// Lifetime: created on (p *pulumi).ProvisionStack entry, torn down when the
// method returns. Plugin clients are RE-USED by the in-engine pluginSession
// (same subprocess, two phases of use).
type preEnginePluginBundle struct {
	mu      sync.Mutex
	log     logger.Logger
	clients map[string]*scPlugin.Client           // name → spawned plugin
	schemas map[string]*scpluginv1.SchemaResponse // name → schema
	byType  map[string]string                     // state-store or secrets type → plugin name
}

func newPreEnginePluginBundle(log logger.Logger) *preEnginePluginBundle {
	return &preEnginePluginBundle{
		log:     log,
		clients: map[string]*scPlugin.Client{},
		schemas: map[string]*scpluginv1.SchemaResponse{},
		byType:  map[string]string{},
	}
}

// Load spawns plugins and populates the bundle. Caller must call Close().
func (b *preEnginePluginBundle) Load(ctx context.Context, specs map[string]api.PluginSpec) error {
	if len(specs) == 0 {
		return nil
	}
	for name, spec := range specs {
		if err := b.loadOne(ctx, name, spec); err != nil {
			return errors.Wrapf(err, "pre-engine load %q", name)
		}
	}
	return nil
}

func (b *preEnginePluginBundle) loadOne(ctx context.Context, name string, spec api.PluginSpec) error {
	if err := verifySHA256(spec); err != nil {
		return err
	}
	hcLog := hclog.New(&hclog.LoggerOptions{
		Name:   "sc-plugin-" + name,
		Level:  hclog.Warn,
		Output: os.Stderr,
	})
	cli, err := scPlugin.NewClient(scPlugin.ClientConfig{
		Path:    spec.Path,
		Managed: true,
		Logger:  hcLog,
	})
	if err != nil {
		return err
	}
	schema, err := cli.Schema(ctx, &scpluginv1.RequestHeader{})
	if err != nil {
		cli.Close()
		return errors.Wrap(err, "schema")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[name] = cli
	b.schemas[name] = schema
	for _, t := range schema.GetStateStoreTypes() {
		if existing, ok := b.byType[t]; ok {
			return errors.Errorf("plugins %q and %q both claim state-store type %q", existing, name, t)
		}
		b.byType[t] = name
	}
	for _, t := range schema.GetSecretsProviderTypes() {
		if existing, ok := b.byType[t]; ok {
			return errors.Errorf("plugins %q and %q both claim secrets-provider type %q", existing, name, t)
		}
		b.byType[t] = name
	}
	b.log.Info(ctx, "pre-engine loaded plugin %q v%s state_store=%v secrets_provider=%v",
		name, schema.GetPluginVersion(),
		schema.GetStateStoreTypes(), schema.GetSecretsProviderTypes())
	return nil
}

// RegisterSidecars installs InitStateStoreFunc sidecars in pApi's in-tree
// map for each plugin-advertised state-store type. SC's existing login
// flow picks them up transparently. The sidecar closures over `b` so
// invocation reuses the already-spawned plugin subprocess.
func (b *preEnginePluginBundle) RegisterSidecars() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, schema := range b.schemas {
		cli := b.clients[name]
		pluginName := name
		for _, t := range schema.GetStateStoreTypes() {
			typeToken := t
			pApi.RegisterInitStateStore(typeToken, func(ctx context.Context, authCfg api.StateStorageConfig, log logger.Logger) error {
				_ = authCfg // v1: config carried via plugin's own env/state, not threaded through
				resp, err := cli.InitStateStore(ctx, &scpluginv1.InitStateStoreRequest{
					Header:         &scpluginv1.RequestHeader{},
					StateStoreType: typeToken,
				})
				if err != nil {
					return errors.Wrapf(err, "plugin %q InitStateStore(%q)", pluginName, typeToken)
				}
				// Apply env vars the plugin asked for.
				for k, v := range resp.GetEnv() {
					if sv := v.GetStringValue(); sv != "" {
						if err := os.Setenv(k, sv); err != nil {
							log.Warn(ctx, "set env %q from plugin %q: %v", k, pluginName, err)
						}
					}
				}
				// Shape backend URL for the Automation API workspace. The
				// fs InitStateStoreFunc sets PULUMI_BACKEND_URL by precedent;
				// mirror that so the plugin-driven path lands in the same place.
				if url := resp.GetBackendUrl(); url != "" {
					if err := os.Setenv("PULUMI_BACKEND_URL", url); err != nil {
						log.Warn(ctx, "set PULUMI_BACKEND_URL from plugin %q: %v", pluginName, err)
					}
				}
				log.Info(ctx, "plugin %q initialised state-store type %q at %q",
					pluginName, typeToken, resp.GetBackendUrl())
				return nil
			})
		}
	}
}

// ClientByResourceType returns the plugin client that owns a resource type
// so the in-engine pluginSession can re-use the already-spawned subprocess
// (shared Phase 2 of the lifecycle).
func (b *preEnginePluginBundle) ClientByResourceType(resourceType string) (*scPlugin.Client, string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, schema := range b.schemas {
		for _, t := range schema.GetResourceTypes() {
			if t == resourceType {
				return b.clients[name], name, true
			}
		}
	}
	return nil, "", false
}

// Clients returns a snapshot of (name, client, schema) tuples for in-engine
// reuse. Caller MUST NOT Close() these — the bundle owns their lifetime.
func (b *preEnginePluginBundle) Clients() []preEnginePluginRef {
	b.mu.Lock()
	defer b.mu.Unlock()
	refs := make([]preEnginePluginRef, 0, len(b.clients))
	for name, cli := range b.clients {
		refs = append(refs, preEnginePluginRef{Name: name, Client: cli, Schema: b.schemas[name]})
	}
	return refs
}

type preEnginePluginRef struct {
	Name   string
	Client *scPlugin.Client
	Schema *scpluginv1.SchemaResponse
}

// Close tears down every plugin subprocess and clears the dispatch tables.
func (b *preEnginePluginBundle) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cli := range b.clients {
		cli.Close()
	}
	b.clients = map[string]*scPlugin.Client{}
	b.schemas = map[string]*scpluginv1.SchemaResponse{}
	b.byType = map[string]string{}
}
