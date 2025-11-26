package api

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"

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
	StackName         string
	ParentEnv         string // parent stack env
	StackEnv          string // current stack env
	ResourceEnv       string // environment where resource should be consumed
	FullReference     string
	DependsOnResource *api.StackConfigDependencyResource
	UsesResource      bool
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
	AddOutput(ctx *sdk.Context, o sdk.Output)
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

// isProductionLikeEnvironment detects production-like environments based on common naming patterns
func isProductionLikeEnvironment(env string) bool {
	if env == "" {
		return false
	}

	env = strings.ToLower(env)

	// Common production environment patterns
	productionPatterns := []string{
		"prod", "production", "live", "prd",
		"main", "master", "release", "stable",
	}

	// Check for exact matches
	for _, pattern := range productionPatterns {
		if env == pattern {
			return true
		}
	}

	// Check for patterns that contain production indicators
	// e.g., "production-eu", "prod-us", "live-west", "main-cluster"
	for _, pattern := range productionPatterns {
		if strings.Contains(env, pattern) {
			return true
		}
	}

	return false
}

// AdoptionProtectionOptions provides comprehensive protection for adopted resources
func AdoptionProtectionOptions(ignoreChanges []string) []sdk.ResourceOption {
	opts := []sdk.ResourceOption{
		// CRITICAL: Protect adopted resources from deletion
		sdk.Protect(true),
	}

	// Add ignore changes if provided
	if len(ignoreChanges) > 0 {
		opts = append(opts, sdk.IgnoreChanges(ignoreChanges))
	}

	return opts
}

// LogAdoptionWarnings logs critical safety warnings for production-like environments
func LogAdoptionWarnings(ctx *sdk.Context, input api.ResourceInput, params ProvisionParams, resourceType, resourceName string) {
	// Always log adoption info
	params.Log.Info(ctx.Context(), "🔄 Adopting %s %q in environment %q", resourceType, resourceName, input.StackParams.Environment)

	// Enhanced warnings for production-like environments
	if isProductionLikeEnvironment(input.StackParams.Environment) {
		params.Log.Warn(ctx.Context(), "🚨 Adopting %s %q in PRODUCTION-LIKE environment %q", resourceType, resourceName, input.StackParams.Environment)
		params.Log.Warn(ctx.Context(), "🛡️  PROTECTION: Resource will be protected from deletion with sdk.Protect(true)")
	} else {
		// Still provide safety information for non-production environments
		params.Log.Info(ctx.Context(), "🛡️  PROTECTION: Resource will be protected from deletion with sdk.Protect(true)")
		params.Log.Info(ctx.Context(), "ℹ️  INFO: Configuration mismatches are ignored to prevent replacements")
	}
}

// ValidateAdoptionConfig validates common adoption configuration requirements
func ValidateAdoptionConfig(adoptFlag bool, resourceName, descriptorName string) error {
	if !adoptFlag {
		return errors.Errorf("adopt flag not set for resource %q", descriptorName)
	}

	if resourceName == "" {
		return errors.Errorf("resource name is required when adopt=true for resource %q", descriptorName)
	}

	return nil
}
