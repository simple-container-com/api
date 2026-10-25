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
)

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
