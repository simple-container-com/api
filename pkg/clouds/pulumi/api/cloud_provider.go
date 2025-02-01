package api

import (
	"reflect"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

const (
	ConfigPassphraseEnvVar  = "PULUMI_CONFIG_PASSPHRASE"
	DefaultPulumiPassphrase = "simple-container.com"
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
	ResourceOutputs     ResourcesOutputs // outputs from dependency resources
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
	ParentEnv     string
	StackEnv      string
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

type (
	PreProcessor   func(any) error
	PreProcessors  map[reflect.Type][]PreProcessor
	PostProcessor  func(any) error
	PostProcessors map[reflect.Type][]PostProcessor
)

type (
	ResourcesOutputs map[string]*api.ResourceOutput
)

type ComputeContext interface {
	SecretEnvVariables() []ComputeEnvVariable
	EnvVariables() []ComputeEnvVariable
	Dependencies() []sdk.Resource
	Outputs() []sdk.Output
	ResolvePlaceholders(obj any) error
	GetPreProcessors(forType any) ([]PreProcessor, bool)
	GetPostProcessors(forType any) ([]PostProcessor, bool)
	RunPreProcessors(forType any, onObject any) error
	RunPostProcessors(forType any, onObject any) error
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
	GetPreProcessors(forType any) ([]PreProcessor, bool)
	AddPreProcessor(forType any, processor PreProcessor)
	GetPostProcessors(forType any) ([]PostProcessor, bool)
	AddPostProcessor(forType any, processor PostProcessor)
	RunPreProcessors(forType any, onObject any) error
	RunPostProcessors(forType any, onObject any) error
}
