package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api/logger"
)

type ProvisionParams struct {
	Provider       sdk.ProviderResource
	Registrar      Registrar
	Log            logger.Logger
	ParentStack    *ParentInfo
	ComputeContext ComputeContext
}

type ParentInfo struct {
	StackName string
	RefString string
}

type ComputeContext interface {
	EnvVariables() map[string]string
	Dependencies() []sdk.Resource
	Outputs() []sdk.Output
}

type ComputeContextCollector interface {
	EnvVariables() map[string]string
	AddEnvVariable(name, value string)
	AddDependency(resource sdk.Resource)
	Dependencies() []sdk.Resource
	AddOutput(o sdk.Output)
	Outputs() []sdk.Output
}
