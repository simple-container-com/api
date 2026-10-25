// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	pulumiVm "github.com/simple-container-com/api/pkg/clouds/pulumi/vm"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// sidecar factories, one per pApi surface. Each produces a function with the
// correct in-tree signature; the function closures over the plugin client +
// a type token, and at call time dispatches to the plugin via gRPC.
//
// In-engine sidecars (Provision, Provider, Registrar, ComputeProcess) require
// the pluginSession's MonitorProxy address — read from the package-level
// activeSession at call time. If activeSession is nil (sidecar called outside
// the inline RunFunc), the sidecar returns a clear error rather than crashing.

// --- state-store sidecar (unchanged shape) ---

func (b *preEnginePluginBundle) makeStateStoreSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.InitStateStoreFunc {
	return func(ctx context.Context, authCfg api.StateStorageConfig, log logger.Logger) error {
		resp, err := cli.InitStateStore(ctx, &scpluginv1.InitStateStoreRequest{
			Header:         &scpluginv1.RequestHeader{},
			StateStoreType: typeToken,
			Config:         serializeConfigToInputs(authCfg),
		})
		if err != nil {
			return errors.Wrapf(err, "plugin %q InitStateStore(%q)", pluginName, typeToken)
		}
		for k, v := range resp.GetEnv() {
			if sv := v.GetStringValue(); sv != "" {
				if err := os.Setenv(k, sv); err != nil {
					log.Warn(ctx, "set env %q from plugin %q: %v", k, pluginName, err)
				}
			}
		}
		if url := resp.GetBackendUrl(); url != "" {
			if err := os.Setenv("PULUMI_BACKEND_URL", url); err != nil {
				log.Warn(ctx, "set PULUMI_BACKEND_URL from plugin %q: %v", pluginName, err)
			}
		}
		log.Info(ctx, "plugin %q initialised state-store type %q at %q", pluginName, typeToken, resp.GetBackendUrl())
		return nil
	}
}

// --- secrets-provider sidecar ---

func (b *preEnginePluginBundle) makeSecretsProviderSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.InitSecretsProviderFunc {
	return func(ctx context.Context, log logger.Logger) (string, string, map[string]string, error) {
		resp, err := cli.InitSecretsProvider(ctx, &scpluginv1.InitSecretsProviderRequest{
			Header:              &scpluginv1.RequestHeader{},
			SecretsProviderType: typeToken,
		})
		if err != nil {
			return "", "", nil, errors.Wrapf(err, "plugin %q InitSecretsProvider(%q)", pluginName, typeToken)
		}
		envMap := map[string]string{}
		for k, v := range resp.GetEnv() {
			if sv := v.GetStringValue(); sv != "" {
				envMap[k] = sv
			}
		}
		log.Info(ctx, "plugin %q initialised secrets-provider type %q (url=%q)", pluginName, typeToken, resp.GetProviderUrl())
		return resp.GetProviderUrl(), resp.GetPassphrase(), envMap, nil
	}
}

// --- resource sidecar (same as existing pluginProvisionFunc) ---

func (b *preEnginePluginBundle) makeResourceSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.ProvisionFunc {
	return func(ctx *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		addr, err := requireMonitorAddr(input.Descriptor.Type)
		if err != nil {
			return nil, err
		}
		stackName, env := stackAndEnv(input)
		resp, err := cli.Provision(ctx.Context(), &scpluginv1.ProvisionRequest{
			Header:       &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stackName, Environment: env, ProviderUrns: activeProviderURNs()},
			ResourceType: typeToken,
			ResourceName: input.Descriptor.Name,
			Inputs:       serializeConfigToInputs(input.Descriptor.Config.Config),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q Provision(%q, %q)", pluginName, typeToken, input.Descriptor.Name)
		}
		return &api.ResourceOutput{Ref: fmt.Sprintf("plugin:%s:%s", pluginName, resp.GetUrn())}, nil
	}
}

// --- provider sidecar: plugin creates pulumi:providers:<pkg> via monitor ---

func (b *preEnginePluginBundle) makeProviderSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.ProvisionFunc {
	return func(ctx *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		addr, err := requireMonitorAddr(input.Descriptor.Type)
		if err != nil {
			return nil, err
		}
		stackName, env := stackAndEnv(input)
		resp, err := cli.Provider(ctx.Context(), &scpluginv1.ProviderRequest{
			Header:               &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stackName, Environment: env},
			ProviderType:         typeToken,
			ProviderResourceName: input.Descriptor.Name,
			Inputs:               serializeConfigToInputs(input.Descriptor.Config.Config),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q Provider(%q)", pluginName, typeToken)
		}
		// Record the provider URN so later resource dispatches see it in
		// RequestHeader.provider_urns — matches 04 §3's auto-pin shape.
		recordProviderURN(typeToken, resp.GetProviderUrn())
		// Return a ref that satisfies initProvider's sdk.ProviderResource cast
		// via a thin wrapper. The pluginProviderResource type carries the URN
		// SC-core-side so downstream code that passes it to Pulumi
		// RegisterResource options routes correctly.
		return &api.ResourceOutput{Ref: newPluginProviderResource(ctx, resp.GetProviderUrn())}, nil
	}
}

// --- registrar sidecar: plugin creates DNS resources via monitor ---

func (b *preEnginePluginBundle) makeRegistrarSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.RegistrarFunc {
	return func(sdkCtx *sdk.Context, desc api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error) {
		addr, err := requireMonitorAddr(typeToken)
		if err != nil {
			return nil, err
		}
		// Serialize the registrar config into the plugin bus input map.
		// Handle both cases: PluginFieldConfig passthrough (for `:`-prefixed
		// types) and typed configs produced when the plugin's /api module
		// registered a provider-config reader (type sans `:`). JSON round-trip
		// handles both uniformly.
		configMap := serializeConfigToInputs(desc.Config.Config)
		resp, err := cli.ProvisionRegistrar(sdkCtx.Context(), &scpluginv1.ProvisionRegistrarRequest{
			Header:        &scpluginv1.RequestHeader{MonitorAddress: addr},
			RegistrarType: typeToken,
			Config:        configMap,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ProvisionRegistrar(%q)", pluginName, typeToken)
		}
		// Sniff static DNS records off the typed config so ProvisionRecords can
		// fan them out — the plugin bus intentionally drops slices on the
		// serialize path (and the Output-bearing api.DnsRecord fields are not
		// JSON-safe anyway; see pkg/api/dns.go DnsRecord ValueOut).
		var staticRecords []api.DnsRecord
		if rc, ok := desc.Config.Config.(api.RegistrarConfig); ok {
			staticRecords = rc.DnsRecords()
		}
		return &pluginRegistrar{
			mainDomain: resp.GetMainDomain(),
			log:        params.Log,
			pluginName: pluginName,
			typeToken:  typeToken,
			cli:        cli,
			records:    staticRecords,
		}, nil
	}
}

// --- compute processor sidecar ---

func (b *preEnginePluginBundle) makeComputeProcessorSidecar(cli *scPlugin.Client, pluginName, typeToken string) pApi.ComputeProcessorFunc {
	return func(ctx *sdk.Context, _ api.Stack, input api.ResourceInput, _ pApi.ComputeContextCollector, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
		addr, err := requireMonitorAddr(typeToken)
		if err != nil {
			return nil, err
		}
		stackName, env := stackAndEnv(input)
		resp, err := cli.ComputeProcess(ctx.Context(), &scpluginv1.ComputeProcessRequest{
			Header:       &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stackName, Environment: env},
			ResourceType: typeToken,
			ResourceName: input.Descriptor.Name,
			Inputs:       serializeConfigToInputs(input.Descriptor.Config.Config),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ComputeProcess(%q)", pluginName, typeToken)
		}
		_ = resp // v1: outputs returned here are available via the resource output; richer wiring (dependency placeholders) in Phase 1b
		return &api.ResourceOutput{Ref: fmt.Sprintf("plugin-compute:%s:%s", pluginName, resp.GetUrn())}, nil
	}
}

// --- vm driver sidecar: plugin creates the machines, core connects to them ---

// makeVmHostsSidecar produces the callable pkg/clouds/pulumi/vm's pluginDriver
// invokes. It adds the two header fields only this side can know — the MonitorProxy
// address the plugin dials to register resources against core's live engine, and the
// provider URNs core has already registered — and forwards everything else verbatim.
//
// Deliberately not a pApi registration: a `type: vm` resource is dispatched by SC's
// own vm resource type and routed to a driver by its `provider:` field, so there is
// no type token for a vm driver to claim. See RegisterSidecars' header note.
func (b *preEnginePluginBundle) makeVmHostsSidecar(cli *scPlugin.Client, pluginName string) pulumiVm.VmHostsCall {
	return func(ctx context.Context, req *scpluginv1.VmHostsRequest) (*scpluginv1.VmHostsResponse, error) {
		addr, err := requireMonitorAddr("vm provider " + req.GetDriver())
		if err != nil {
			return nil, err
		}
		if req.Header == nil {
			req.Header = &scpluginv1.RequestHeader{}
		}
		req.Header.MonitorAddress = addr
		req.Header.ProviderUrns = activeProviderURNs()
		return cli.VmHosts(ctx, req)
	}
}

// --- pre-destroy hook sidecar ---

func (b *preEnginePluginBundle) makePreDestroySidecar(cli *scPlugin.Client, pluginName string) pApi.PreDestroyHookFunc {
	return func(ctx context.Context, _ api.Stack, _ api.DestroyParams, _ auto.Stack, log logger.Logger) {
		if _, err := cli.PreDestroy(ctx, &scpluginv1.PreDestroyRequest{
			Header: &scpluginv1.RequestHeader{},
		}); err != nil {
			log.Warn(ctx, "plugin %q PreDestroy: %v", pluginName, err)
		}
	}
}

// --- shared helpers ---

func stackAndEnv(input api.ResourceInput) (string, string) {
	if input.StackParams == nil {
		return "", ""
	}
	return input.StackParams.StackName, input.StackParams.Environment
}

func requireMonitorAddr(typeToken string) (string, error) {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if activeSession == nil {
		return "", errors.Errorf("plugin dispatch for %q requires an active plugin session (no inline RunFunc running)", typeToken)
	}
	activeSession.mu.Lock()
	addr := activeSession.proxyAddr
	activeSession.mu.Unlock()
	if addr == "" {
		return "", errors.Errorf("plugin monitor proxy not started yet (StartProxy skipped) — dispatch for %q cannot proceed", typeToken)
	}
	return addr, nil
}

// activeProviderURNs returns a snapshot of provider URNs the active session
// has recorded via Provider sidecar invocations. Used in RequestHeader for
// downstream Provision calls.
func activeProviderURNs() map[string]string {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if activeSession == nil {
		return nil
	}
	activeSession.mu.Lock()
	defer activeSession.mu.Unlock()
	if len(activeSession.providerURNs) == 0 {
		return nil
	}
	out := make(map[string]string, len(activeSession.providerURNs))
	for k, v := range activeSession.providerURNs {
		out[k] = v
	}
	return out
}

func recordProviderURN(providerType, urn string) {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if activeSession == nil {
		return
	}
	activeSession.mu.Lock()
	defer activeSession.mu.Unlock()
	if activeSession.providerURNs == nil {
		activeSession.providerURNs = map[string]string{}
	}
	activeSession.providerURNs[providerType] = urn
}

// serializeConfigToInputs converts an arbitrary config value (either an
// api.PluginFieldConfig or a plugin-typed struct the plugin's /api module
// registered) to the plugin bus's InputValue map via JSON round-trip. Only
// string-valued fields propagate for v1; richer types land in Phase 1c.
func serializeConfigToInputs(cfg any) map[string]*scpluginv1.InputValue {
	out := map[string]*scpluginv1.InputValue{}
	if cfg == nil {
		return out
	}
	// JSON round-trip to map[string]any.
	b, err := json.Marshal(cfg)
	if err != nil {
		return out
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return out
	}
	for k, v := range raw {
		switch sv := v.(type) {
		case string:
			out[k] = &scpluginv1.InputValue{Value: &scpluginv1.InputValue_StringValue{StringValue: sv}}
		case bool:
			out[k] = &scpluginv1.InputValue{Value: &scpluginv1.InputValue_BoolValue{BoolValue: sv}}
		case float64:
			out[k] = &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: sv}}
		}
	}
	return out
}

// pluginRegistrar is the SC-core-side wrapper around a plugin's Registrar
// surface. v1 implements the methods pApi.Registrar requires with the plugin
// bus's current ProvisionRegistrar RPC; richer per-method RPCs land in a
// Phase 1b proto bump. Methods that would need per-call RPCs return a clear
// "not yet supported" error.
type pluginRegistrar struct {
	mainDomain string
	log        logger.Logger
	pluginName string
	typeToken  string
	cli        *scPlugin.Client
	// records captures the registrar's static dnsRecords block from server.yaml
	// so ProvisionRecords can fan them out to per-record NewDnsRecord RPCs.
	// Populated at ProvisionRegistrar time by sniffing an api.RegistrarConfig
	// interface on the typed config.
	records []api.DnsRecord
}

func (r *pluginRegistrar) MainDomain() string { return r.mainDomain }

// ProvisionRecords iterates the registrar's static dnsRecords and dispatches
// one per-record NewRecord call to the plugin. This mirrors what the in-tree
// cloudflare / cloudru / yandex registrars do and preserves state shape — each
// record lands as its own Pulumi resource with the plugin's naming scheme.
func (r *pluginRegistrar) ProvisionRecords(ctx *sdk.Context, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if len(r.records) == 0 {
		return &api.ResourceOutput{Ref: fmt.Sprintf("plugin-registrar:%s", r.pluginName)}, nil
	}
	refs := make([]any, 0, len(r.records))
	for _, record := range r.records {
		out, err := r.NewRecord(ctx, record)
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ProvisionRecords(%q)", r.pluginName, record.Name)
		}
		refs = append(refs, out.Ref)
	}
	return &api.ResourceOutput{Ref: refs}, nil
}

// NewRecord — Phase 1b. For a plain-string record.Value we dispatch
// synchronously; otherwise we chain via ApplyT so the plugin receives a
// resolved string. The synchronous path matters: when ProvisionRecords
// discards the returned ref (as the in-tree registrars do too), an unforced
// Output chain never fires, so the gRPC call never happens and no record
// resource gets registered.
func (r *pluginRegistrar) NewRecord(ctx *sdk.Context, record api.DnsRecord) (*api.ResourceOutput, error) {
	addr, err := requireMonitorAddr(r.typeToken)
	if err != nil {
		return nil, err
	}
	if record.Value != "" {
		resp, err := r.cli.NewDnsRecord(ctx.Context(), &scpluginv1.NewDnsRecordRequest{
			Header:        &scpluginv1.RequestHeader{MonitorAddress: addr},
			RegistrarType: r.typeToken,
			RecordName:    record.Name,
			RecordType:    record.Type,
			ValueResolved: record.Value,
			Proxied:       record.Proxied,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q NewDnsRecord(%q)", r.pluginName, record.Name)
		}
		return &api.ResourceOutput{Ref: resp.GetUrn()}, nil
	}
	urnOut := record.ValueOut.ApplyT(func(resolved string) (string, error) {
		resp, err := r.cli.NewDnsRecord(ctx.Context(), &scpluginv1.NewDnsRecordRequest{
			Header:        &scpluginv1.RequestHeader{MonitorAddress: addr},
			RegistrarType: r.typeToken,
			RecordName:    record.Name,
			RecordType:    record.Type,
			ValueResolved: resolved,
			Proxied:       record.Proxied,
		})
		if err != nil {
			return "", errors.Wrapf(err, "plugin %q NewDnsRecord(%q)", r.pluginName, record.Name)
		}
		return resp.GetUrn(), nil
	})
	return &api.ResourceOutput{Ref: urnOut}, nil
}

// NewOverrideHeaderRule — Phase 1b. Resolves rule.ToHost via ApplyT.
func (r *pluginRegistrar) NewOverrideHeaderRule(ctx *sdk.Context, stack api.Stack, rule pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	addr, err := requireMonitorAddr(r.typeToken)
	if err != nil {
		return nil, err
	}
	toHost := rule.ToHost
	if toHost == nil {
		toHost = sdk.String("")
	}
	urnOut := toHost.ToStringOutput().ApplyT(func(resolvedToHost string) (string, error) {
		req := &scpluginv1.NewOverrideHeaderRuleRequest{
			Header:         &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stack.Name},
			RegistrarType:  r.typeToken,
			StackName:      stack.Name,
			RuleName:       rule.Name,
			FromHost:       rule.FromHost,
			ToHostResolved: resolvedToHost,
			PathPrefix:     rule.PathPrefix,
		}
		if rule.BasicAuth != nil {
			req.BasicAuth = &scpluginv1.BasicAuthMsg{
				Username: rule.BasicAuth.Username,
				Password: rule.BasicAuth.Password,
				Realm:    rule.BasicAuth.Realm,
			}
		}
		if rule.OverridePages != nil {
			req.OverridePages = &scpluginv1.OverridePagesMsg{
				IndexPage:    rule.OverridePages.IndexPage,
				NotFoundPage: rule.OverridePages.NotFoundPage,
			}
		}
		resp, err := r.cli.NewOverrideHeaderRule(ctx.Context(), req)
		if err != nil {
			return "", errors.Wrapf(err, "plugin %q NewOverrideHeaderRule(%q)", r.pluginName, rule.Name)
		}
		return resp.GetUrn(), nil
	})
	return &api.ResourceOutput{Ref: urnOut}, nil
}

// ProvisionDomainForEndpoint — Phase 1b. Resolves endpoint.TargetHost via ApplyT.
func (r *pluginRegistrar) ProvisionDomainForEndpoint(ctx *sdk.Context, stack api.Stack, endpoint pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	addr, err := requireMonitorAddr(r.typeToken)
	if err != nil {
		return nil, err
	}
	target := endpoint.TargetHost
	if target == nil {
		target = sdk.String("")
	}
	urnOut := target.ToStringOutput().ApplyT(func(resolvedTarget string) (string, error) {
		resp, err := r.cli.ProvisionDomainForEndpoint(ctx.Context(), &scpluginv1.ProvisionDomainForEndpointRequest{
			Header:             &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stack.Name},
			RegistrarType:      r.typeToken,
			StackName:          stack.Name,
			EndpointName:       endpoint.Name,
			Domain:             endpoint.Domain,
			TargetHostResolved: resolvedTarget,
			Websocket:          endpoint.WebSocket,
		})
		if err != nil {
			return "", errors.Wrapf(err, "plugin %q ProvisionDomainForEndpoint(%q)", r.pluginName, endpoint.Domain)
		}
		return resp.GetUrn(), nil
	})
	return &api.ResourceOutput{Ref: urnOut}, nil
}

// pluginProviderResource is a thin shim that satisfies sdk.ProviderResource
// for a provider the plugin registered against the monitor. The SC-core-side
// cast at pkg/clouds/pulumi/provision.go:336 accepts this because it
// implements the ProviderResource interface via embedding ResourceState.
type pluginProviderResource struct {
	sdk.ProviderResourceState
	urn string
}

func (p *pluginProviderResource) URN() sdk.URNOutput {
	return sdk.String(p.urn).ToStringOutput().ApplyT(func(s string) sdk.URN {
		return sdk.URN(s)
	}).(sdk.URNOutput)
}

func newPluginProviderResource(_ *sdk.Context, urn string) sdk.ProviderResource {
	// Return an opaque reference. SC's initProvider only uses the Ref downstream
	// via its URN; the ResourceState embedding keeps type-checks happy without
	// needing to actually construct a Pulumi provider resource in SC core.
	return &pluginProviderResource{urn: urn}
}
