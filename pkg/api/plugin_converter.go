// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
)

// PluginStackConverter is the injection point SC core's parse-time path
// uses to turn a plugin-owned template's merged StackConfig* into the
// plugin's typed Input via the ConvertStackConfig RPC. The dialling
// code lives in pkg/stackread (which imports pkg/plugin — allowed) and
// registers itself here with RegisterPluginStackConverter at init-time.
//
// Why a function variable instead of calling pkg/plugin directly: pkg/api
// cannot import pkg/plugin (pkg/plugin already imports pkg/api — the
// cycle is why stackread exists as a leaf). The seam keeps pkg/api
// clean while letting Prepare*ForDeploy branch on plugin-owned types.
type PluginStackConverter func(ctx context.Context, pluginName string, req PluginStackConvertRequest) (*PluginStackConvertResult, error)

// PluginStackConvertRequest carries the operator-side state the
// plugin's existing /api-registered ToXxxConfig closures already
// consume: the typed StackConfig (compose/single-image/static), the
// stack name, and the ${git:root}-resolved stackDir. All scalar — no
// SC-core-side types that cannot cross a gRPC boundary.
type PluginStackConvertRequest struct {
	TemplateType string
	StackName    string
	StackDir     string
	// StackConfig is one of *StackConfigCompose, *StackConfigSingleImage,
	// or *StackConfigStatic. stackread marshals it to the proto's
	// map<string, InputValue> wire form.
	StackConfig any
	// TemplateConfig is the parent-side TEMPLATE config body — the raw
	// map or typed struct `stackDesc.Config.Config` carries after
	// DetectTemplateType ran. For plugin-owned templates it is a raw
	// map[string]any (the opaque decoder preserved the YAML tree); the
	// plugin decodes it into its own typed TemplateConfig on receipt.
	TemplateConfig any
}

// PluginStackConvertResult mirrors the typed response SC core wraps in
// pluginPreparedInput. The three downstream interfaces
// (DnsConfigAware, ResourceAware, WithDependsOnResources) read exactly
// these fields.
type PluginStackConvertResult struct {
	// Input is the plugin's typed Input (*LambdaInput /
	// *StaticSiteInput / *EcsFargateInput etc.) decoded from the RPC
	// response. SC core forwards it verbatim through the Provision RPC
	// — the plugin's Provision handler receives the same shape it
	// returned here, keeping the end-to-end round-trip stable.
	Input any
	// BaseDnsZone satisfies DnsConfigAware.OverriddenBaseZone.
	BaseDnsZone string
	// Uses satisfies ResourceAware.Uses.
	Uses []string
	// DependsOnResources satisfies WithDependsOnResources.DependsOnResources.
	DependsOnResources []StackConfigDependencyResource
}

var pluginStackConverter PluginStackConverter

// RegisterPluginStackConverter installs the dialling implementation
// (currently stackread.ConvertPluginStackConfig). SC core distributions
// that never spawn plugins (schema-gen, test fixtures that don't
// declare plugins:) can leave this nil — the Prepare*ForDeploy
// fallback is to try the in-tree cloud*ConverterMapping first and only
// reach for a plugin converter when the type is plugin-owned AND the
// converter is registered. A nil converter on a plugin-owned template
// is a configuration error, not a crash.
func RegisterPluginStackConverter(c PluginStackConverter) {
	pluginStackConverter = c
}

// pluginPreparedInput wraps a PluginStackConvertResult as a Go value
// that satisfies DnsConfigAware, ResourceAware, and
// WithDependsOnResources — the three interfaces SC core type-asserts
// off clientStackDesc.Config.Config at pkg/clouds/pulumi/deploy.go
// (lines 133, 146, 153) and pkg/provisioner/unresolved.go (109, 112).
//
// Also satisfies the no-op side of each interface for stacks whose
// plugin response omitted the structured fields (empty []string / nil
// slice reads as "no constraints" — same as a stack that never
// declared the attribute).
type pluginPreparedInput struct {
	result *PluginStackConvertResult
}

// NewPluginPreparedInput wraps a plugin's typed Input + the three
// structured fields for downstream type-asserts. The pkg/stackread
// converter calls this after decoding the ConvertStackConfigResponse.
func NewPluginPreparedInput(result *PluginStackConvertResult) *pluginPreparedInput {
	return &pluginPreparedInput{result: result}
}

// OverriddenBaseZone implements DnsConfigAware.
func (p *pluginPreparedInput) OverriddenBaseZone() string {
	if p == nil || p.result == nil {
		return ""
	}
	return p.result.BaseDnsZone
}

// Uses implements ResourceAware.
func (p *pluginPreparedInput) Uses() []string {
	if p == nil || p.result == nil {
		return nil
	}
	return p.result.Uses
}

// DependsOnResources implements WithDependsOnResources.
func (p *pluginPreparedInput) DependsOnResources() []StackConfigDependencyResource {
	if p == nil || p.result == nil {
		return nil
	}
	return p.result.DependsOnResources
}

// Input returns the plugin's typed Input so callers that need to
// forward it onwards (e.g. the Provision RPC marshal) can reach it.
// The field type is any because SC core does NOT inspect it — the
// plugin owns both ends of the round-trip.
func (p *pluginPreparedInput) Input() any {
	if p == nil || p.result == nil {
		return nil
	}
	return p.result.Input
}

// convertPluginOwnedTemplate branches off Prepare*ForDeploy when the
// template type is plugin-owned. Returns (desc, true) on hit,
// (nil, false) on miss (type not plugin-owned, or no converter
// registered). The caller then decides whether a nil converter is an
// error (a plugin-owned type with no dialler is unrecoverable at the
// deploy stage).
func convertPluginOwnedTemplate(ctx context.Context, stackDir, stackName, parentStack string, tpl StackDescriptor, stackConfig any) (*StackDescriptor, bool, error) {
	binding, ok := IsPluginOwnedType(tpl.Type)
	if !ok {
		return nil, false, nil
	}
	if pluginStackConverter == nil {
		return nil, true, errPluginConverterNotRegistered(tpl.Type, binding.PluginName)
	}
	result, err := pluginStackConverter(ctx, binding.PluginName, PluginStackConvertRequest{
		TemplateType:   tpl.Type,
		StackName:      stackName,
		StackDir:       stackDir,
		StackConfig:    stackConfig,
		TemplateConfig: tpl.Config.Config,
	})
	if err != nil {
		return nil, true, err
	}
	return &StackDescriptor{
		Type:        tpl.Type,
		ParentStack: parentStack,
		Config:      Config{Config: NewPluginPreparedInput(result)},
	}, true, nil
}
