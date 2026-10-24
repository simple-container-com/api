// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
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
		_ = authCfg
		resp, err := cli.InitStateStore(ctx, &scpluginv1.InitStateStoreRequest{
			Header:         &scpluginv1.RequestHeader{},
			StateStoreType: typeToken,
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
		resp, err := cli.ProvisionRegistrar(sdkCtx.Context(), &scpluginv1.ProvisionRegistrarRequest{
			Header:        &scpluginv1.RequestHeader{MonitorAddress: addr},
			RegistrarType: typeToken,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ProvisionRegistrar(%q)", pluginName, typeToken)
		}
		return &pluginRegistrar{
			mainDomain: resp.GetMainDomain(),
			log:        params.Log,
			pluginName: pluginName,
			typeToken:  typeToken,
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
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ComputeProcess(%q)", pluginName, typeToken)
		}
		_ = resp // v1: outputs returned here are available via the resource output; richer wiring (dependency placeholders) in Phase 1b
		return &api.ResourceOutput{Ref: fmt.Sprintf("plugin-compute:%s:%s", pluginName, resp.GetUrn())}, nil
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
}

func (r *pluginRegistrar) MainDomain() string { return r.mainDomain }

func (r *pluginRegistrar) ProvisionRecords(ctx *sdk.Context, _ pApi.ProvisionParams) (*api.ResourceOutput, error) {
	r.log.Debug(ctx.Context(), "pluginRegistrar(%q).ProvisionRecords — plugin handles its own records in ProvisionRegistrar", r.pluginName)
	return &api.ResourceOutput{Ref: fmt.Sprintf("plugin-registrar:%s", r.pluginName)}, nil
}

func (r *pluginRegistrar) NewRecord(_ *sdk.Context, _ api.DnsRecord) (*api.ResourceOutput, error) {
	return nil, errors.Errorf("pluginRegistrar.NewRecord not yet wired for plugin %q (Phase 1b)", r.pluginName)
}

func (r *pluginRegistrar) NewOverrideHeaderRule(_ *sdk.Context, _ api.Stack, _ pApi.OverrideHeaderRule) (*api.ResourceOutput, error) {
	return nil, errors.Errorf("pluginRegistrar.NewOverrideHeaderRule not yet wired for plugin %q (Phase 1b)", r.pluginName)
}

func (r *pluginRegistrar) ProvisionDomainForEndpoint(_ *sdk.Context, _ api.Stack, _ pApi.DomainEndpoint) (*api.ResourceOutput, error) {
	return nil, errors.Errorf("pluginRegistrar.ProvisionDomainForEndpoint not yet wired for plugin %q (Phase 1b)", r.pluginName)
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
