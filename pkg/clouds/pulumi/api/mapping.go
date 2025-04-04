package api

import (
	"context"

	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

type (
	ProvisionFunc        func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params ProvisionParams) (*api.ResourceOutput, error)
	ComputeProcessorFunc func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector ComputeContextCollector, params ProvisionParams) (*api.ResourceOutput, error)
	RegistrarFunc        func(sdkCtx *sdk.Context, desc api.RegistrarDescriptor, params ProvisionParams) (Registrar, error)
	InitStateStoreFunc   func(ctx context.Context, authCfg api.StateStorageConfig, log logger.Logger) error
)

var (
	InitStateStoreFuncByType   = map[string]InitStateStoreFunc{}
	ProviderFuncByType         = map[string]ProvisionFunc{}
	ProvisionFuncByType        = map[string]ProvisionFunc{}
	RegistrarFuncByType        = map[string]RegistrarFunc{}
	ComputeProcessorFuncByType = map[string]ComputeProcessorFunc{}
)

func RegisterInitStateStore(providerType string, fnc InitStateStoreFunc) {
	InitStateStoreFuncByType = lo.Assign(InitStateStoreFuncByType, map[string]InitStateStoreFunc{
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
