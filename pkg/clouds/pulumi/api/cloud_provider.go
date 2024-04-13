package api

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

type ProvisionParams struct {
	// normally required to be present
	Provider       sdk.ProviderResource
	Registrar      Registrar
	Log            logger.Logger
	ComputeContext ComputeContext
	// optionally set on per-case basis
	DnsPreference     *DnsPreference
	ParentStack       *ParentInfo
	StackDescriptor   *api.StackDescriptor
	UseResources      map[string]bool
	DependOnResources []api.StackConfigDependResource
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
	AddDependencyTplExtension(depName string, resName string, values map[string]string)
}
