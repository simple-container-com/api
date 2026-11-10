// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

type (
	ProvisionFunc           func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params ProvisionParams) (*api.ResourceOutput, error)
	ComputeProcessorFunc    func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector ComputeContextCollector, params ProvisionParams) (*api.ResourceOutput, error)
	RegistrarFunc           func(sdkCtx *sdk.Context, desc api.RegistrarDescriptor, params ProvisionParams) (Registrar, error)
	InitStateStoreFunc      func(ctx context.Context, authCfg api.StateStorageConfig, log logger.Logger) error
	InitSecretsProviderFunc func(ctx context.Context, log logger.Logger) (secretsProviderURL string, passphrase string, env map[string]string, err error)
	PreDestroyHookFunc      func(ctx context.Context, stack api.Stack, params api.DestroyParams, stackSource auto.Stack, log logger.Logger)
)

var (
	InitStateStoreFuncByType      = map[string]InitStateStoreFunc{}
	InitSecretsProviderFuncByType = map[string]InitSecretsProviderFunc{}
	ProviderFuncByType            = map[string]ProvisionFunc{}
	ProvisionFuncByType           = map[string]ProvisionFunc{}
	RegistrarFuncByType           = map[string]RegistrarFunc{}
	ComputeProcessorFuncByType    = map[string]ComputeProcessorFunc{}
	PreDestroyHookFuncs           []PreDestroyHookFunc

	// PluginResourceTypes names every resource / provider / compute-processor
	// / registrar type that a plugin has advertised in its Schema. SC core's
	// configureResource and getProvisionParams consult it to decide whether
	// to run initProvider — plugin-managed types own their provider
	// construction internally, in the plugin's own pulumi.Context, since a
	// ProviderResource handle doesn't cross pctx boundaries. See
	// forge/docs/design/2026-10-24/sc-plugin-architecture/04 Addendum H.
	PluginResourceTypes = map[string]struct{}{}

	// ComponentResourceTypes names every resource type a plugin has
	// advertised as an SC Component in its Schema
	// (SchemaResponse.component_resource_types, slice 1a of the
	// sc-components-platform milestone).
	//
	// The sidecar wrapper in plugin_sidecars.go consults this to decide
	// whether to run its template-dispatch pre-step before the plugin's
	// Provision RPC fires: a Component resource's config carries a
	// `deployTo:` key naming a template, and core dispatches that template
	// with plugin-supplied overrides merged over the operator's template
	// config.
	//
	// Populated at plugin-load time by preEnginePluginBundle.RegisterSidecars.
	// Set-membership is the dispatch discriminator — NOT a Go-interface
	// assertion on the config value, which can't match for plugin-owned
	// types in production (see pkg/api/component_config.go header + the
	// operator memory interface_assertion_against_plugin_owned_value_fails_in_prod).
	//
	// Design: forge/docs/design/2026-11-10/sc-components-platform/ §P1.
	ComponentResourceTypes = map[string]struct{}{}

	// ComponentAllowedOverrides records, per Component resource type, the
	// `.Raw` keys a plugin may set on its dispatched template's config.
	// Populated at plugin-load time from
	// SchemaResponse.component_allowed_overrides. Parse-time validation
	// (pkg/api/plugin_requirements.go) refuses any override not on this
	// list; the sidecar wrapper enforces the same as defense-in-depth.
	//
	// Starter allowlist for most components:
	// [image, env, ingressRoutes, imagePullSecretRef]. Operator-locked
	// by omission: vpc, subnets, iam, anything that would let a plugin
	// re-point the operator's compute at an attacker VPC.
	ComponentAllowedOverrides = map[string][]string{}
)

// RegisterComponentResourceType marks a resource type as an SC Component.
// Called by preEnginePluginBundle.RegisterSidecars for every entry in
// the plugin's SchemaResponse.component_resource_types.
func RegisterComponentResourceType(resourceType string, allowedOverrides []string) {
	if resourceType == "" {
		return
	}
	ComponentResourceTypes[resourceType] = struct{}{}
	if allowedOverrides != nil {
		ComponentAllowedOverrides[resourceType] = append([]string(nil), allowedOverrides...)
	}
}

// IsComponentResourceType reports whether a resource type is an SC
// Component.
func IsComponentResourceType(resourceType string) bool {
	_, ok := ComponentResourceTypes[resourceType]
	return ok
}

// IsComponentOverrideAllowed reports whether a plugin may set the given
// `.Raw` key as an override on the Component resource type's dispatched
// template config.
func IsComponentOverrideAllowed(resourceType, key string) bool {
	for _, k := range ComponentAllowedOverrides[resourceType] {
		if k == key {
			return true
		}
	}
	return false
}

// RegisterPluginType marks a resource type as plugin-dispatched. Called
// by preEnginePluginBundle.RegisterSidecars for every type the plugin
// advertises in Schema (resource / provider / compute-processor). SC
// core's provision flow uses this to skip initProvider for the type.
func RegisterPluginType(resourceType string) {
	if resourceType == "" {
		return
	}
	PluginResourceTypes[resourceType] = struct{}{}
}

// IsPluginType reports whether a resource type is plugin-dispatched.
func IsPluginType(resourceType string) bool {
	_, ok := PluginResourceTypes[resourceType]
	return ok
}

func RegisterInitStateStore(providerType string, fnc InitStateStoreFunc) {
	InitStateStoreFuncByType = lo.Assign(InitStateStoreFuncByType, map[string]InitStateStoreFunc{
		providerType: fnc,
	})
}

func RegisterInitSecretsProvider(providerType string, fnc InitSecretsProviderFunc) {
	InitSecretsProviderFuncByType = lo.Assign(InitSecretsProviderFuncByType, map[string]InitSecretsProviderFunc{
		providerType: fnc,
	})
}

func RegisterProvider(providerType string, fnc ProvisionFunc) {
	ProviderFuncByType = lo.Assign(ProviderFuncByType, map[string]ProvisionFunc{
		providerType: fnc,
	})
}

func RegisterResources(register map[string]ProvisionFunc) {
	ProvisionFuncByType = lo.Assign(ProvisionFuncByType, register)
}

func RegisterRegistrar(providerType string, fnc RegistrarFunc) {
	RegistrarFuncByType = lo.Assign(RegistrarFuncByType, map[string]RegistrarFunc{
		providerType: fnc,
	})
}

func RegisterComputeProcessor(register map[string]ComputeProcessorFunc) {
	ComputeProcessorFuncByType = lo.Assign(ComputeProcessorFuncByType, register)
}

func RegisterPreDestroyHook(fnc PreDestroyHookFunc) {
	PreDestroyHookFuncs = append(PreDestroyHookFuncs, fnc)
}
