// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrPluginUnimplementedConversion is the sentinel the stackread
// converter returns when a plugin's binary predates the
// ConvertStackConfig RPC (gRPC Unimplemented). SC core treats this
// as "the plugin cannot answer the RPC yet" and falls through to
// the pre-A2 cloud*ConverterMapping path, which is still populated
// via each plugin's /api/init.go blank-imports — strictly additive.
//
// Rationale: new RPCs on the SCCloudPlugin service must be forward-
// and backward-compatible, so a CLI release that adds a required
// RPC does not break every deployed stack whose pinned plugin
// binary predates it. This sentinel is the mechanism; the standing
// rule for every future new RPC is to catch Unimplemented at the
// dial site and fall through to a known-good legacy path (if one
// exists) or return a legible operator error (if not).
//
// Peer-flagged root cause of the ConvertStackConfig fleet break
// 2026-11-02: api #535 shipped the caller + no plugin release had
// the handler + forge's aws pin predated the handler. Every
// isolated-matrix deploy hit "ConvertStackConfig: Unimplemented".
// With this fallback, that failure mode becomes silent: the plugin
// says Unimplemented, SC core uses the typed-converter mapping,
// deploy proceeds as it did before A2.
var ErrPluginUnimplementedConversion = errors.New("plugin does not implement ConvertStackConfig")

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

// PluginPreparedInput wraps a PluginStackConvertResult as a Go value
// that satisfies DnsConfigAware, ResourceAware, and
// WithDependsOnResources — the three interfaces SC core type-asserts
// off clientStackDesc.Config.Config at pkg/clouds/pulumi/deploy.go
// (lines 133, 146, 153) and pkg/provisioner/unresolved.go (109, 112).
//
// EVERY FIELD MUST BE EXPORTED. SC core walks this struct via
// reflection in pkg/provisioner/placeholders (applyTemplatesRecursive)
// for `${X}` placeholder substitution, and that walker calls
// reflect.Value.Set on every field. A reflect.Set into an unexported
// field panics with "reflect.Value.Set using value obtained using
// unexported field" — live-caught on forge-aigateway isolated run
// 41469779418 (2026-11-02), which panicked before any resource was
// provisioned and failed every service in the matrix.
//
// Also satisfies the no-op side of each interface for stacks whose
// plugin response omitted the structured fields (empty []string / nil
// slice reads as "no constraints" — same as a stack that never
// declared the attribute).
//
// Note that method NAMES collide with the fields they expose, so the
// fields have an underscore suffix. The interface methods thunk to
// the fields and are what Go code sees via type-assertion; the fields
// are what the placeholders walker sees via reflection.
type PluginPreparedInput struct {
	// Input is the plugin's typed Input (decoded back into a
	// map[string]any by pkg/stackread via scPlugin.InputsToPlain so
	// the reflect walker never has to deal with unexported fields the
	// plugin's typed struct might have carried).
	Input any `yaml:"input,omitempty" json:"input,omitempty"`
	// BaseDnsZone_ is the typed Input's api.DnsConfigAware.OverriddenBaseZone.
	BaseDnsZone_ string `yaml:"baseDnsZone,omitempty" json:"baseDnsZone,omitempty"`
	// Uses_ is the typed Input's api.ResourceAware.Uses.
	Uses_ []string `yaml:"uses,omitempty" json:"uses,omitempty"`
	// DependsOnResources_ is the typed Input's
	// api.WithDependsOnResources.DependsOnResources.
	DependsOnResources_ []StackConfigDependencyResource `yaml:"dependsOnResources,omitempty" json:"dependsOnResources,omitempty"`
}

// NewPluginPreparedInput wraps a plugin's typed Input + the three
// structured fields for downstream type-asserts. The pkg/stackread
// converter calls this after decoding the ConvertStackConfigResponse.
func NewPluginPreparedInput(result *PluginStackConvertResult) *PluginPreparedInput {
	if result == nil {
		return &PluginPreparedInput{}
	}
	return &PluginPreparedInput{
		Input:               result.Input,
		BaseDnsZone_:        result.BaseDnsZone,
		Uses_:               result.Uses,
		DependsOnResources_: result.DependsOnResources,
	}
}

// OverriddenBaseZone implements DnsConfigAware.
func (p *PluginPreparedInput) OverriddenBaseZone() string {
	if p == nil {
		return ""
	}
	return p.BaseDnsZone_
}

// Uses implements ResourceAware.
func (p *PluginPreparedInput) Uses() []string {
	if p == nil {
		return nil
	}
	return p.Uses_
}

// DependsOnResources implements WithDependsOnResources.
func (p *PluginPreparedInput) DependsOnResources() []StackConfigDependencyResource {
	if p == nil {
		return nil
	}
	return p.DependsOnResources_
}

// MarshalJSON unwraps PluginPreparedInput for the Provision bus. SC
// core forwards the deploying stack's `Config.Config` across the bus
// via pkg/clouds/pulumi/plugin_sidecars.go's serializeConfigToInputs,
// which calls json.Marshal. The plugin's Provision handler decodes
// the resulting InputValue tree into its typed Input shape
// (*LambdaInput, *StaticSiteInput, …). Without this method, the
// serialised shape would be `{input: {...real Input fields...},
// baseDnsZone: ..., uses: [...], dependsOnResources: [...]}` — and
// the plugin's decoder would find every expected Input field at the
// top level is nil, panic'ing on first dereference. Live-caught on
// forge-aigateway isolated-stack run 41483738313 (2026-11-02).
//
// Returning just p.Input makes the wire shape identical to what SC
// core's pre-A2 Prepare*ForDeploy produced: the raw typed Input is
// what the plugin's Provision handler expects.
//
// SC core's own interface asserts (DnsConfigAware / ResourceAware /
// WithDependsOnResources) are still answered by the methods above —
// those read the exported *_ fields, which are preserved on the
// SC-core-side wrapper. Only the WIRE representation unwraps.
func (p *PluginPreparedInput) MarshalJSON() ([]byte, error) {
	if p == nil || p.Input == nil {
		return []byte("null"), nil
	}
	return json.Marshal(p.Input)
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
		if errors.Is(err, ErrPluginUnimplementedConversion) {
			// Plugin binary predates the RPC. Fall through to the
			// pre-A2 cloud*ConverterMapping path by returning
			// isPluginOwned=false — the caller's branch at
			// pkg/api/client.go treats this as a non-plugin-owned
			// template and uses the typed-converter mapping that
			// the plugin's /api/init.go populated at Go-link time.
			// Standing rule — see ErrPluginUnimplementedConversion
			// doc for the shape every future new RPC should wear.
			return nil, false, nil
		}
		return nil, true, err
	}
	return &StackDescriptor{
		Type:        tpl.Type,
		ParentStack: parentStack,
		Config:      Config{Config: NewPluginPreparedInput(result)},
	}, true, nil
}
