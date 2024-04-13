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
	DnsPreference  *DnsPreference
	SkipRefresh    bool
}

type DnsPreference struct {
	BaseZone string
}
type ParentInfo struct {
	StackName    string
	FulReference string
}

type ComputeEnvVariable struct {
	Name         string
	Value        string
	ResourceName string
	ResourceType string
	StackName    string
}

type ComputeContext interface {
	EnvVariables() []ComputeEnvVariable
	Dependencies() []sdk.Resource
	Outputs() []sdk.Output
	ResolvePlaceholders(obj any) error
}

type ComputeContextCollector interface {
	EnvVariables() []ComputeEnvVariable
	AddEnvVariableIfNotExist(name, value, resType, resName, stackName string)
	AddDependency(resource sdk.Resource)
	Dependencies() []sdk.Resource
	AddOutput(o sdk.Output)
	Outputs() []sdk.Output
	ResolvePlaceholders(obj any) error
	AddResourceTplExtension(resName string, value map[string]string)
}
