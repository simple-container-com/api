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
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	scVm "github.com/simple-container-com/api/pkg/clouds/vm"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// logStreamConsumer runs in a goroutine for each loaded plugin. Opens the
// plugin's LogStream and routes incoming events into the bundle logger
// with a [plugin %q] prefix. Ends when ctx is cancelled (bundle Close)
// or the plugin exits (stream EOF).
func logStreamConsumer(ctx context.Context, cli *scPlugin.Client, pluginName string, log logger.Logger) {
	stream, err := cli.LogStream(ctx, &scpluginv1.LogStreamRequest{PluginName: pluginName})
	if err != nil {
		log.Debug(ctx, "plugin %q LogStream open: %v", pluginName, err)
		return
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			if ctx.Err() == nil {
				log.Debug(ctx, "plugin %q LogStream closed: %v", pluginName, err)
			}
			return
		}
		prefix := "[plugin " + pluginName
		if ev.GetPhase() != "" {
			prefix += "/" + ev.GetPhase()
		}
		prefix += "] "
		msg := prefix + ev.GetMessage()
		switch ev.GetSeverity() {
		case scpluginv1.LogSeverity_LOG_SEVERITY_DEBUG:
			log.Debug(ctx, "%s", msg)
		case scpluginv1.LogSeverity_LOG_SEVERITY_WARN:
			log.Warn(ctx, "%s", msg)
		case scpluginv1.LogSeverity_LOG_SEVERITY_ERROR:
			log.Error(ctx, "%s", msg)
		default:
			log.Info(ctx, "%s", msg)
		}
	}
}

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
	// logCtx governs the LogStream consumer goroutine spawned per plugin
	// after loadOne. Close() cancels this ctx so the consumers return and
	// the stream RPC ends cleanly on the plugin side.
	logCtx    context.Context
	logCancel context.CancelFunc
}

func newPreEnginePluginBundle(log logger.Logger) *preEnginePluginBundle {
	ctx, cancel := context.WithCancel(context.Background())
	return &preEnginePluginBundle{
		log:       log,
		clients:   map[string]*scPlugin.Client{},
		schemas:   map[string]*scpluginv1.SchemaResponse{},
		byType:    map[string]string{},
		logCtx:    ctx,
		logCancel: cancel,
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
	// Resolve the binary — either spec.Path verbatim, or fetch from the
	// release URL + verify sha256. Honours spec.Source for the download path.
	resolvedPath, err := scPlugin.ResolveBinaryPath(spec, scPlugin.DefaultCacheRoot)
	if err != nil {
		return errors.Wrap(err, "resolve plugin binary")
	}
	// SHA256 was checked by ResolveBinaryPath for source-driven loads; for
	// path-driven loads, verify here (verifySHA256 only runs when sha256 is set).
	spec.Path = resolvedPath
	if err := verifySHA256(spec); err != nil {
		return err
	}
	hcLog := hclog.New(&hclog.LoggerOptions{
		Name:   "sc-plugin-" + name,
		Level:  hclog.Warn,
		Output: os.Stderr,
	})
	cli, err := scPlugin.NewClient(scPlugin.ClientConfig{
		Path:    resolvedPath,
		Managed: true,
		Logger:  hcLog,
		// Wire the composite-plugin broker. Each spawned plugin gets its
		// own SCCore endpoint; a PeerInvoke call on it dispatches by
		// plugin name back into THIS bundle's registry. The dispatcher
		// captures the bundle so lookups route to all plugins loaded in
		// this process, including those that spawn after this one — the
		// lookup is lazy, done at call time.
		Peer: &bundlePeerDispatcher{bundle: b},
	})
	if err != nil {
		return err
	}
	schema, err := cli.Schema(ctx, &scpluginv1.RequestHeader{})
	if err != nil {
		cli.Close()
		return errors.Wrap(err, "schema")
	}
	// Start the LogStream consumer BEFORE acquiring the bundle lock so a
	// slow stream open doesn't serialise plugin loads.
	go logStreamConsumer(b.logCtx, cli, name, b.log)
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

// RegisterSidecars installs sidecars in pApi's in-tree registration maps
// for every plugin-advertised type. SC's existing dispatch picks them up
// transparently — the same path in-tree types use.
//
// Covered surfaces (Phase 1 complete):
//   - state_store_types       → pApi.InitStateStoreFuncByType
//   - secrets_provider_types  → pApi.InitSecretsProviderFuncByType
//   - resource_types          → pApi.ProvisionFuncByType
//   - provider_types          → pApi.ProviderFuncByType
//   - registrar_types         → pApi.RegistrarFuncByType
//   - compute_processor_types → pApi.ComputeProcessorFuncByType
//   - any plugin              → pApi.PreDestroyHookFuncs (appended once per plugin)
//   - vm_drivers              → scVm.RegisterPluginDriver for the config tier
//     (capabilities, and therefore validation), plus pulumiVm.RegisterPluginClient
//     for the Pulumi tier (the VmHosts callable). Two registrations and one
//     driver — see RegisterPluginClient for why the second is keyed by plugin and
//     not by driver.
//
// Sidecars closure over `b` so invocation reuses the already-spawned plugin
// subprocesses — one plugin binary serves all its advertised surfaces across
// the full ProvisionStack / DeployStack / DestroyStack / PreviewStack call.
// Monitor address for in-engine calls (Provision, Provider, Registrar,
// ComputeProcess) is read from the active pluginSession via
// sessionMonitorAddr(); if the session is not up (e.g. called before the
// inline RunFunc), the sidecar returns a clear error.
//
// Returns an error rather than logging one, because one of the registrations it
// performs can genuinely fail: two installed plugins claiming the same vm driver is
// an operator-fixable configuration error that must not resolve silently — which
// plugin won would decide which cloud a machine is created in.
func (b *preEnginePluginBundle) RegisterSidecars() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, schema := range b.schemas {
		cli := b.clients[name]
		pluginName := name

		// State-storage: pre-engine, no monitor needed.
		for _, t := range schema.GetStateStoreTypes() {
			typeToken := t
			pApi.RegisterInitStateStore(typeToken, b.makeStateStoreSidecar(cli, pluginName, typeToken))
		}

		// Secrets-provider: pre-engine, no monitor needed.
		for _, t := range schema.GetSecretsProviderTypes() {
			typeToken := t
			pApi.RegisterInitSecretsProvider(typeToken, b.makeSecretsProviderSidecar(cli, pluginName, typeToken))
		}

		// Resource types: in-engine, needs monitor address. Also tagged in
		// PluginResourceTypes so provision.go's getProvisionParams knows to
		// skip initProvider — the plugin owns provider construction in its
		// own pctx (ProviderResource handles don't cross pctx boundaries).
		resMap := map[string]pApi.ProvisionFunc{}
		for _, t := range schema.GetResourceTypes() {
			typeToken := t
			resMap[typeToken] = b.makeResourceSidecar(cli, pluginName, typeToken)
			pApi.RegisterPluginType(typeToken)
		}
		if len(resMap) > 0 {
			pApi.RegisterResources(resMap)
		}

		// Provider types: in-engine, needs monitor address.
		for _, t := range schema.GetProviderTypes() {
			typeToken := t
			pApi.RegisterProvider(typeToken, b.makeProviderSidecar(cli, pluginName, typeToken))
		}

		// Registrar types: in-engine, needs monitor address.
		for _, t := range schema.GetRegistrarTypes() {
			typeToken := t
			pApi.RegisterRegistrar(typeToken, b.makeRegistrarSidecar(cli, pluginName, typeToken))
		}

		// Compute processor types: in-engine, needs monitor address.
		cpMap := map[string]pApi.ComputeProcessorFunc{}
		for _, t := range schema.GetComputeProcessorTypes() {
			typeToken := t
			cpMap[typeToken] = b.makeComputeProcessorSidecar(cli, pluginName, typeToken)
		}
		if len(cpMap) > 0 {
			pApi.RegisterComputeProcessor(cpMap)
		}

		// VM drivers: in-engine, needs monitor address. Unlike every surface above,
		// this one does NOT go through a pApi registration map — a `type: vm`
		// resource is dispatched by SC's own vm resource type and then routed by its
		// `provider:` field, so the two registrations are into the vm tiers
		// themselves (see the header note).
		for _, spec := range schema.GetVmDrivers() {
			if err := scVm.RegisterPluginDriver(pluginName, spec.GetDriver(), scVm.DriverInfoFromProto(spec)); err != nil {
				return err
			}
		}
		if len(schema.GetVmDrivers()) > 0 {
			pulumiVm.RegisterPluginClient(pluginName, b.makeVmHostsSidecar(cli, pluginName))
		}

		// PreDestroy hook: one per plugin, no type keying — plugin chooses
		// internally what to clean up based on request_type at call time.
		pApi.RegisterPreDestroyHook(b.makePreDestroySidecar(cli, pluginName))
	}
	return nil
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
	if b.logCancel != nil {
		b.logCancel()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cli := range b.clients {
		cli.Close()
	}
	// The vm registrations are package-level in the vm tiers, so they outlive the
	// bundle unless dropped here. Dispatching to an exited subprocess is the failure
	// this prevents: a second provision in the same process would find the driver row
	// and the callable still present and hang on a dead gRPC connection.
	scVm.ResetPluginDrivers()
	pulumiVm.ResetPluginClients()
	b.clients = map[string]*scPlugin.Client{}
	b.schemas = map[string]*scpluginv1.SchemaResponse{}
	b.byType = map[string]string{}
}
