// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package pulumi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

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
	return func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
		addr, err := requireMonitorAddr(input.Descriptor.Type)
		if err != nil {
			return nil, err
		}
		if err := b.provisionDomainRegistrar(ctx, pluginName, input, params); err != nil {
			return nil, err
		}
		// Use the api.Stack argument's Name (the childStack name — e.g.
		// "forge-notifier--staging") rather than input.StackParams.StackName
		// (the CLI --stack value — e.g. "forge-notifier"). In-tree drivers
		// receive stack.Name = "<project>--<env>" (via toChildStack +
		// StackNameInEnv in pkg/clouds/pulumi/provisioner.go). An in-plugin
		// resource that names itself off stack.Name — ECR repo, Lambda
		// function, IAM role, API Gateway — would otherwise produce a
		// URN missing the env suffix, breaking the no-destroy invariant.
		stackName := stack.Name
		if stackName == "" {
			stackName, _ = stackAndEnv(input)
		}
		_, env := stackAndEnv(input)
		cliStack, _ := stackAndEnv(input)
		req := &scpluginv1.ProvisionRequest{
			Header: &scpluginv1.RequestHeader{
				MonitorAddress: addr,
				// The Pulumi engine's log endpoint. Without it ctx.Log on
				// the plugin's own pctx nil-derefs — pulumi-aws calls
				// ctx.Log.Warn in resolveDeployImageRef's ApplyT when a
				// deploy would ship by tag, and the panic in that
				// background goroutine exits status 2 with no visible
				// trace. The Pulumi engine exports this env var on every
				// RunFunc, so SC core can read it directly.
				EngineAddress: engineAddr(),
				StackName:     stackName,
				// Parent-stack name — see RequestHeader.cli_stack_name in
				// plugin.proto. The plugin uses this to shape
				// `input.StackParams.StackName` for drivers whose state
				// URN carries PARENT prefix (aws secrets, env-var fan-out)
				// while `stack_name` keeps CHILD for the ECR/Lambda URN
				// family #516 fixed.
				CliStackName: cliStack,
				Environment:  env,
				ProjectName:  ctx.Project(),
				ProviderUrns: activeProviderURNs(),
			},
			ResourceType: typeToken,
			ResourceName: input.Descriptor.Name,
			Inputs:       serializeConfigToInputs(input.Descriptor.Config.Config),
			// A compute template (a client stack's deploy) reads the env its
			// `uses:` resources contributed from params.ComputeContext. Without
			// this a plugin template got none of it — no MONGO_*, no bucket env.
			ComputeEnvVars: computeContextToProto(params.ComputeContext),
		}
		// If the deploying stack has an active registrar backed by a plugin,
		// pack its coordinates so the receiving plugin can proxy DNS back via
		// SCCore.PeerInvoke (D8). pApi.Registrar is a cross-plugin service;
		// without this a plugin's Provision that calls params.Registrar.*
		// nil-dereferences.
		if pr := resolvePluginRegistrar(ctx, params); pr != nil {
			req.RegistrarPluginName = pr.pluginName
			req.RegistrarType = pr.typeToken
			req.RegistrarMainDomain = pr.mainDomain
		}
		resp, err := cli.Provision(ctx.Context(), req)
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q Provision(%q, %q)", pluginName, typeToken, input.Descriptor.Name)
		}
		// Drain buffer: after the plugin's Provision returns, pulumi-aws
		// (and any other driver with lazy ApplyT chains) still has
		// background goroutines whose closures fire on outputs that
		// resolve during preview finalisation — Function URL,
		// RepositoryUrl, Image.ImageUri. Those goroutines call
		// RegisterResource against the monitor that SC core owns.
		// Returning from this sidecar too fast lets SC core's deploy
		// program progress into finalisation while the plugin's
		// goroutines are mid-registration; the engine then detects
		// a dropped RPC and errors with
		// "waiting for RPCs: error reading from server: EOF".
		//
		// pApi.Registrar's own methods already return promptly because
		// the registrar plugin is a different process; the race is
		// specifically with the driver's self-registered sub-resources.
		//
		// The buffer is bounded so a legitimate slow plugin surfaces as
		// a timeout rather than silent hang. 10s is comfortably above
		// what pulumi-aws's typical Lambda provisioning needs post-RPC.
		// This is option B in the plugin-drain plan; the proper fix
		// (an AwaitDrain RPC the plugin implements with ctx.wait()) is
		// a follow-up — option C.
		time.Sleep(provisionDrainBuffer)
		return &api.ResourceOutput{Ref: fmt.Sprintf("plugin:%s:%s", pluginName, resp.GetUrn())}, nil
	}
}

// provisionDrainBuffer is how long the sidecar waits after cli.Provision
// returns before letting SC core's deploy move on. See the comment
// at the call site above for the full rationale.
var provisionDrainBuffer = 10 * time.Second

// provisionDomainRegistrar makes sure the plugin holds the registrar for the client
// stack's `domain:` before its Provision runs.
//
// With several registrars core builds each one lazily, the first time it writes a
// record in that zone itself. A plugin compute template publishes the stack's domain
// on its own side, with a registrar it received over ProvisionRegistrar — and since
// core never touched that zone, it never sent one: the yandex plugin answered a
// yc-serverless-container with `domain: x.simple-forge.ru` with "no registrar for
// domain" (and before sc-plugin-yandex#6, a nil-pointer panic).
//
// Only a registrar of a type THIS plugin serves is resolved. A domain whose zone
// belongs to another registrar (e.g. Cloudflare in front of a YC container) is left
// for the plugin to refuse with its own error, and core does not construct a
// registrar the deploy would otherwise never have touched.
func (b *preEnginePluginBundle) provisionDomainRegistrar(ctx *sdk.Context, pluginName string, input api.ResourceInput, params pApi.ProvisionParams) error {
	multi, ok := params.Registrar.(*multiRegistrar)
	if !ok {
		// A single registrar is built eagerly by initRegistrar, so the plugin already has it.
		return nil
	}
	domain := stackConfigDomain(input.Descriptor.Config.Config)
	if domain == "" {
		return nil
	}
	regType, err := multi.RegistrarTypeFor(domain)
	if err != nil || !b.pluginServesRegistrarType(pluginName, regType) {
		return nil
	}
	if _, err := multi.RegistrarFor(ctx, domain); err != nil {
		return errors.Wrapf(err, "failed to provision registrar %q for domain %q to plugin %q", regType, domain, pluginName)
	}
	return nil
}

func (b *preEnginePluginBundle) pluginServesRegistrarType(pluginName, regType string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	schema, ok := b.schemas[pluginName]
	if !ok {
		return false
	}
	for _, t := range schema.GetRegistrarTypes() {
		if t == regType {
			return true
		}
	}
	return false
}

// stackConfigDomain reads `stackConfig.domain` off a compute template's input — the
// shape SC core converts a client stack into (the template's config plus the client's
// whole stackConfig). It goes through JSON, as the bus does, since the input is a
// plugin-typed struct core has no static type for.
func stackConfigDomain(cfg any) string {
	if cfg == nil {
		return ""
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	var probe struct {
		StackConfig struct {
			Domain string `json:"domain"`
		} `json:"stackConfig"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return ""
	}
	return probe.StackConfig.Domain
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
		// Forward the deploying stack's preferred base zone. An in-tree
		// registrar reads this straight off params.DnsPreference; a plugin one
		// can only see it if we put it on the wire. Dropping it made the
		// catch-all registrar's configured zone win, so a green deploy wrote
		// the record into the wrong zone and DELETED the live one — see
		// base_dns_zone in plugin.proto.
		var baseDnsZone string
		if params.DnsPreference != nil {
			baseDnsZone = params.DnsPreference.BaseZone
		}
		resp, err := cli.ProvisionRegistrar(sdkCtx.Context(), &scpluginv1.ProvisionRegistrarRequest{
			Header:        &scpluginv1.RequestHeader{MonitorAddress: addr},
			RegistrarType: typeToken,
			Config:        configMap,
			BaseDnsZone:   baseDnsZone,
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
	return func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
		addr, err := requireMonitorAddr(typeToken)
		if err != nil {
			return nil, err
		}
		// Pass the child stack name (`forge-notifier--staging`), not
		// `input.StackParams.StackName` (the CLI --stack value — e.g.
		// `forge-notifier`). The in-tree ComputeProcessor + its initProvider
		// baked `stack.Name` into every resource and the provider URN; a plugin
		// that reads the parent name instead produces a byte-different URN and
		// Pulumi plans a destroy of the live mongodb provider +
		// DatabaseUser + random password on first deploy that moves the type
		// to the plugin bus. Mirrors the same guard the resource sidecar got
		// in #516.
		stackName := stack.Name
		if stackName == "" {
			stackName, _ = stackAndEnv(input)
		}
		_, env := stackAndEnv(input)

		// ParentInfo crosses as JSON — the Go struct carries a nested
		// *api.StackConfigDependencyResource whose shape evolves faster than
		// a hand-written proto message, and the plugin unmarshals back into
		// the same type via api.ParentInfo. Absent / zero ParentStack means
		// no parent (byo context), which is fine on the wire as an empty
		// field.
		var parentInfoJSON []byte
		if params.ParentStack != nil {
			parentInfoJSON, err = json.Marshal(params.ParentStack)
			if err != nil {
				return nil, errors.Wrapf(err, "plugin %q ComputeProcess(%q): marshal ParentStack", pluginName, typeToken)
			}
		}

		resp, err := cli.ComputeProcess(ctx.Context(), &scpluginv1.ComputeProcessRequest{
			Header:         &scpluginv1.RequestHeader{MonitorAddress: addr, StackName: stackName, Environment: env},
			ResourceType:   typeToken,
			ResourceName:   input.Descriptor.Name,
			Inputs:         serializeConfigToInputs(input.Descriptor.Config.Config),
			ParentInfoJson: parentInfoJSON,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "plugin %q ComputeProcess(%q)", pluginName, typeToken)
		}

		// Apply the ComputeContextCollector mutations the plugin asked for.
		// AddEnvVariableIfNotExist / AddSecretEnvVariableIfNotExist and the
		// two tpl-extension methods are the only `pApi.ComputeContextCollector`
		// mutators that are serialisable — the rest (sdk.Resource handles,
		// Go closures) stay in-plugin and are applied against the plugin's
		// own sdk.Context before Build returns. See pkg/plugin/compute.go.
		if collector != nil {
			for _, v := range resp.GetEnvVars() {
				if v.GetSecret() {
					collector.AddSecretEnvVariableIfNotExist(v.GetName(), v.GetValue(), v.GetResourceType(), v.GetResourceName(), v.GetStackName())
				} else {
					collector.AddEnvVariableIfNotExist(v.GetName(), v.GetValue(), v.GetResourceType(), v.GetResourceName(), v.GetStackName())
				}
			}
			for _, t := range resp.GetTplExtensions() {
				switch t.GetKind() {
				case scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_RESOURCE:
					collector.AddResourceTplExtension(t.GetResName(), t.GetValues())
				case scpluginv1.TemplateExtensionKind_TEMPLATE_EXTENSION_KIND_DEPENDENCY:
					collector.AddDependencyTplExtension(t.GetDepName(), t.GetResName(), t.GetValues())
				}
			}
		}

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
	return func(ctx context.Context, stack api.Stack, params api.DestroyParams, stackSource auto.Stack, log logger.Logger) {
		// Pack stack outputs as a map<string, bytes>. mongodb's
		// DropDatabaseHook (and its successors) reads these filtered by
		// key suffix. auto.OutputValue is JSON-friendly (`{"value": ...,
		// "secret": bool}`); encode each one individually so a plugin can
		// unmarshal only the keys it cares about rather than paying the
		// cost of decoding every output.
		outputs := map[string][]byte{}
		// Guard the stackSource.Outputs call: a zero-value auto.Stack (seen
		// in unit tests, and theoretically possible when the pre-destroy
		// hook runs against a stack that was never provisioned) panics
		// rather than returning an error. defer-recover keeps the hook
		// running on whatever we DO have — client descriptor + destroy
		// params — rather than taking the whole deploy down.
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warn(ctx, "plugin %q PreDestroy: read stack outputs panicked: %v", pluginName, r)
				}
			}()
			raw, err := stackSource.Outputs(ctx)
			if err != nil {
				log.Warn(ctx, "plugin %q PreDestroy: read stack outputs: %v", pluginName, err)
				return
			}
			for key, v := range raw {
				b, err := json.Marshal(v)
				if err != nil {
					log.Warn(ctx, "plugin %q PreDestroy: marshal output %q: %v", pluginName, key, err)
					continue
				}
				outputs[key] = b
			}
		}()

		// The per-env client descriptor. Smaller than packing the whole
		// api.Stack (which carries every env), targeted enough to extend.
		var clientDescriptorJSON []byte
		if stack.Client.Stacks != nil {
			if desc, ok := stack.Client.Stacks[params.Environment]; ok {
				if b, err := json.Marshal(desc); err == nil {
					clientDescriptorJSON = b
				} else {
					log.Warn(ctx, "plugin %q PreDestroy: marshal client descriptor: %v", pluginName, err)
				}
			}
		}

		// DestroyParams. StackParams is embedded; both are plain JSON.
		destroyParamsJSON, err := json.Marshal(params)
		if err != nil {
			log.Warn(ctx, "plugin %q PreDestroy: marshal destroy params: %v", pluginName, err)
		}

		if _, err := cli.PreDestroy(ctx, &scpluginv1.PreDestroyRequest{
			Header:               &scpluginv1.RequestHeader{StackName: stack.Name, Environment: params.Environment},
			StackOutputs:         outputs,
			ClientDescriptorJson: clientDescriptorJSON,
			DestroyParamsJson:    destroyParamsJSON,
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

// engineAddr returns the active session's EngineProxy address, or an empty
// string when no session is attached. Unlike requireMonitorAddr this is
// best-effort — a plugin that doesn't need ctx.Log (no-op driver) runs
// fine without an engine conn, so an empty return is not an error.
func engineAddr() string {
	activeSessionMu.Lock()
	defer activeSessionMu.Unlock()
	if activeSession == nil {
		return ""
	}
	activeSession.mu.Lock()
	defer activeSession.mu.Unlock()
	return activeSession.engineProxyAddr
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
// registered) to the plugin bus's InputValue map via JSON round-trip.
//
// Nested objects and lists travel as InputMap / InputList. Until they did, only
// the top-level scalars reached a plugin, which silently dropped every
// structured input — for a compute template, the client stack's whole
// `stackConfig` (image, env, secrets, uses, domain, cloudExtras). The plugin
// side reverses this with plugin.InputsToPlain. JSON numbers arrive as doubles;
// a plugin decoding into an int field gets the integral value back.
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
		if iv := toInputValue(v); iv != nil {
			out[k] = iv
		}
	}
	return out
}

// toInputValue maps one decoded-JSON value onto the bus's InputValue. A JSON
// null returns nil and the key is omitted, which a plugin decoding into a struct
// reads as the zero value — the same thing the null meant.
func toInputValue(v any) *scpluginv1.InputValue {
	switch sv := v.(type) {
	case string:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_StringValue{StringValue: sv}}
	case bool:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_BoolValue{BoolValue: sv}}
	case float64:
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_DoubleValue{DoubleValue: sv}}
	case map[string]any:
		fields := make(map[string]*scpluginv1.InputValue, len(sv))
		for k, item := range sv {
			if iv := toInputValue(item); iv != nil {
				fields[k] = iv
			}
		}
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_MapValue{MapValue: &scpluginv1.InputMap{Fields: fields}}}
	case []any:
		items := make([]*scpluginv1.InputValue, 0, len(sv))
		for _, item := range sv {
			// A list keeps its positions: a null element becomes an empty
			// InputValue (decoded back as nil) rather than vanishing and
			// shifting every later element.
			iv := toInputValue(item)
			if iv == nil {
				iv = &scpluginv1.InputValue{}
			}
			items = append(items, iv)
		}
		return &scpluginv1.InputValue{Value: &scpluginv1.InputValue_ListValue{ListValue: &scpluginv1.InputList{Items: items}}}
	default:
		return nil
	}
}

// computeContextToProto serialises a compute context's env (plain and secret)
// for ProvisionRequest.compute_env_vars. Only the env crosses: Dependencies()
// are live sdk.Resource handles of core's own context and cannot be sent.
func computeContextToProto(cc pApi.ComputeContext) []*scpluginv1.ComputeEnvVariable {
	if cc == nil {
		return nil
	}
	plain, secret := cc.EnvVariables(), cc.SecretEnvVariables()
	if len(plain)+len(secret) == 0 {
		return nil
	}
	out := make([]*scpluginv1.ComputeEnvVariable, 0, len(plain)+len(secret))
	add := func(vars []pApi.ComputeEnvVariable, isSecret bool) {
		for _, v := range vars {
			out = append(out, &scpluginv1.ComputeEnvVariable{
				Name:  v.Name,
				Value: v.Value,
				// The collector keeps secrets in their own list and does not
				// always set the flag on the entry; the list is authoritative.
				Secret:       isSecret || v.Secret,
				ResourceName: v.ResourceName,
				ResourceType: v.ResourceType,
				StackName:    v.StackName,
			})
		}
	}
	add(plain, false)
	add(secret, true)
	return out
}

// resolvePluginRegistrar peeks at ProvisionParams.Registrar and returns
// the active *pluginRegistrar authoritative for the deploying stack's
// base DNS zone, so the sidecar can pack its coordinates into
// ProvisionRequest for the receiving plugin to proxy DNS calls back
// via PeerInvoke (D8).
//
//   - For a plain *pluginRegistrar: return it directly.
//   - For a *multiRegistrar: resolve via its forDomain against the
//     stack's preferred base zone (DnsPreference), then type-assert.
//   - For an in-tree registrar: return nil — in-tree registrars have no
//     plugin bus endpoint and callers that need one must declare one.
func resolvePluginRegistrar(ctx *sdk.Context, params pApi.ProvisionParams) *pluginRegistrar {
	switch r := params.Registrar.(type) {
	case *pluginRegistrar:
		return r
	case *multiRegistrar:
		if r == nil {
			return nil
		}
		baseZone := ""
		if params.DnsPreference != nil {
			baseZone = params.DnsPreference.BaseZone
		}
		if baseZone == "" {
			return nil
		}
		delegate, err := r.forDomain(baseZone)
		if err != nil {
			return nil
		}
		built, err := r.build(ctx, delegate)
		if err != nil {
			return nil
		}
		if pr, ok := built.(*pluginRegistrar); ok {
			return pr
		}
		return nil
	default:
		return nil
	}
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
		// Wrapped rather than returned bare: a registrar's Ref is an Output in
		// every other implementation (cloudru hands back cmd.ID(), the in-tree
		// cloudflare handed back the record resource's ID), and callers rely on
		// that — gcp/static_website.go exports it straight through an
		// sdk.Output assertion. The dispatch above already happened, so this
		// costs nothing and keeps the two paths of this method interchangeable.
		return &api.ResourceOutput{Ref: sdk.String(resp.GetUrn()).ToStringOutput()}, nil
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
