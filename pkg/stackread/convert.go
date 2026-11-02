// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package stackread

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pkg/errors"

	"github.com/simple-container-com/api/pkg/api"
	scPlugin "github.com/simple-container-com/api/pkg/plugin"
	scpluginv1 "github.com/simple-container-com/api/pkg/plugin/proto/v1"
)

// pluginRegistry maps a plugin NAME (as declared in server.yaml's
// plugins: block) to the PluginSpec that was used to resolve the
// binary. Populated by DiscoverPluginTypes so the A2 converter can
// re-spawn by name later. Package-level state because A2's dial-site
// (pkg/api.PluginStackConverter) is called from a deeper call-stack
// that does not thread the ServerDescriptor through.
var pluginSpecsByName = map[string]api.PluginSpec{}

// recordSpecForConverter is called by DiscoverPluginTypes for each
// declared plugin so convertStackConfigViaPlugin can re-spawn by name.
func recordSpecForConverter(name string, spec api.PluginSpec) {
	pluginSpecsByName[name] = spec
}

// init wires the pkg/api dialling seam. Package stackread registers
// its ConvertStackConfig implementation at init() so that any caller
// that imports stackread automatically gets the A2 branch wired up.
// Callers that never import stackread (schema-gen, test helpers) use
// the pre-A2 cloud*ConverterMapping path uncontested — exactly the
// opt-in shape pkg/api.RegisterPluginStackConverter documents.
func init() {
	api.RegisterPluginStackConverter(convertStackConfigViaPlugin)
}

// convertStackConfigViaPlugin dials the plugin's ConvertStackConfig
// RPC. Short-lived — spawns, calls, closes. The parse-time two-pass
// does not keep plugins alive, and the preengine (ProvisionStack)
// re-spawns from the same binary when the deploy itself runs; a
// single client's lifetime per RPC keeps the resource accounting
// simple without serialising all dials.
func convertStackConfigViaPlugin(ctx context.Context, pluginName string, req api.PluginStackConvertRequest) (*api.PluginStackConvertResult, error) {
	spec, ok := pluginSpecsByName[pluginName]
	if !ok {
		return nil, errors.Errorf("stackread: no plugin spec recorded for plugin %q — did the pre-parse run?", pluginName)
	}

	cli, _, err := scPlugin.ResolveAndSchema(ctx, pluginName, spec, scPlugin.DefaultCacheRoot, nil, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "stackread: dial plugin %q for ConvertStackConfig", pluginName)
	}
	defer cli.Close()

	rpcReq := &scpluginv1.ConvertStackConfigRequest{
		Header:         &scpluginv1.RequestHeader{StackName: req.StackName},
		TemplateType:   req.TemplateType,
		StackName:      req.StackName,
		StackDir:       req.StackDir,
		StackConfig:    scPlugin.PlainToInputs(req.StackConfig),
		TemplateConfig: scPlugin.PlainToInputs(req.TemplateConfig),
	}

	resp, err := cli.ConvertStackConfig(ctx, rpcReq)
	if err != nil {
		// A plugin binary that predates the RPC answers gRPC
		// Unimplemented (the generated UnimplementedSCCloudPluginServer
		// shim). SC core treats this as "plugin cannot answer yet" and
		// falls through to the pre-A2 cloud*ConverterMapping path —
		// peer-flagged standing rule from forge-escalation-resolve
		// after the 2026-11-02 ConvertStackConfig fleet break. New
		// RPCs on SCCloudPlugin must be additive by construction, not
		// discipline.
		if status.Code(err) == codes.Unimplemented {
			return nil, api.ErrPluginUnimplementedConversion
		}
		return nil, errors.Wrapf(err, "stackread: plugin %q ConvertStackConfig", pluginName)
	}

	result := &api.PluginStackConvertResult{
		BaseDnsZone: resp.GetBaseDnsZone(),
		Uses:        resp.GetUses(),
	}
	// Plugin returns the typed Input as a scalar InputValue tree. Decode
	// back into a map[string]any so SC core's Provision RPC marshal path
	// (serializeConfigToInputs) can re-marshal it cleanly on the next
	// RPC — same JSON-round-trip shape both sides agree on.
	result.Input = scPlugin.InputsToPlain(resp.GetInput())

	// depends_on_resources carries full StackConfigDependencyResource
	// shapes as JSON for forward-compat (same technique
	// ComputeProcessRequest.parent_info_json uses).
	for _, raw := range resp.GetDependsOnResourcesJson() {
		var entry api.StackConfigDependencyResource
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, errors.Wrap(err, "stackread: decode depends_on_resources entry")
		}
		result.DependsOnResources = append(result.DependsOnResources, entry)
	}
	return result, nil
}
