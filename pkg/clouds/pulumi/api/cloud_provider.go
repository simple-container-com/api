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
	DependencyProviders map[string]DependencyProvider
	DnsPreference       *DnsPreference
	ParentStack         *ParentInfo
	StackDescriptor     *api.StackDescriptor
	UseResources        map[string]bool
	DependOnResources   []api.StackConfigDependencyResource
	BaseEnvVariables    map[string]string
	HelpersImage        string
}

type DependencyProvider struct {
	Provider sdk.ProviderResource
	Config   api.Config
}

type DnsPreference struct {
	BaseZone string
}
type ParentInfo struct {
	StackName     string
	FullReference string
}

type ComputeEnvVariable struct {
	Name         string
	Value        string
	ResourceName string
	ResourceType string
	StackName    string
	Secret       bool
}

type ComputeContext interface {
	SecretEnvVariables() []ComputeEnvVariable
	EnvVariables() []ComputeEnvVariable
	Dependencies() []sdk.Resource
	Outputs() []sdk.Output
	ResolvePlaceholders(obj any) error
}

type ComputeContextCollector interface {
	SecretEnvVariables() []ComputeEnvVariable
	EnvVariables() []ComputeEnvVariable
	AddEnvVariableIfNotExist(name, value, resType, resName, stackName string)
	AddSecretEnvVariableIfNotExist(name, value, resType, resName, stackName string)
	AddDependency(resource sdk.Resource)
	Dependencies() []sdk.Resource
	AddOutput(o sdk.Output)
	Outputs() []sdk.Output
	ResolvePlaceholders(obj any) error
	AddResourceTplExtension(resName string, value map[string]string)
	AddDependencyTplExtension(depName string, resName string, values map[string]string)
}
