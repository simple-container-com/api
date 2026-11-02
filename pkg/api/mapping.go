// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/compose"
)

type ConfigReaderFunc func(config *Config) (Config, error)

type ProvisionerInitFunc func(config Config, opts ...ProvisionerOption) (Provisioner, error)

type (
	CloudHelperInitFunc func(opts ...CloudHelperOption) (CloudHelper, error)
	ConfigRegisterMap   map[string]ConfigReaderFunc
)

type (
	ProvisionerRegisterMap  map[string]ProvisionerInitFunc
	CloudHelpersRegisterMap map[CloudHelperType]CloudHelperInitFunc
)

var providerConfigMapping = ConfigRegisterMap{}

var provisionerConfigMapping = ProvisionerRegisterMap{}

var cloudHelpersConfigMapping = CloudHelpersRegisterMap{}

type (
	ProvisionerFieldConfigReadFunc   func(config *Config) (Config, error)
	ProvisionerFieldConfigRegister   map[string]ProvisionerFieldConfigReadFunc
	ProvisionerFieldConfigReaderFunc func(cType string, c *Config) (Config, error)
	ToCloudComposeConvertFunc        func(tpl any, composeCfg compose.Config, stackCfg *StackConfigCompose) (any, error)
	CloudComposeConfigRegister       map[string]ToCloudComposeConvertFunc
	ToCloudSingleImageConvertFunc    func(tpl any, stackCfg *StackConfigSingleImage) (any, error)
	CloudSingleImageConfigRegister   map[string]ToCloudSingleImageConvertFunc
	ToCloudStaticSiteConvertFunc     func(tpl any, rootDir, stackName string, stackCfg *StackConfigStatic) (any, error)
	CloudStaticSiteConfigRegister    map[string]ToCloudStaticSiteConvertFunc
)

var (
	provisionerFieldConfigMapping    = ProvisionerFieldConfigRegister{}
	cloudComposeConverterMapping     = CloudComposeConfigRegister{}
	cloudSingleImageConverterMapping = CloudSingleImageConfigRegister{}
	cloudStaticSiteConverterMapping  = CloudStaticSiteConfigRegister{}
)

func ConvertDescriptor[T any](from any, to *T) (*T, error) {
	if bytes, err := yaml.Marshal(from); err == nil {
		if err = yaml.Unmarshal(bytes, to); err != nil {
			return nil, err
		} else {
			return to, nil
		}
	} else {
		return nil, err
	}
}

func ConvertConfig[T any](config *Config, to *T) (Config, error) {
	res, err := ConvertDescriptor(config.Config, to)
	config.Config = res
	return *config, err
}

func ConvertAuth[T any](auth AuthConfig, creds *T) error {
	if err := json.Unmarshal([]byte(auth.CredentialsValue()), creds); err != nil {
		return err
	} else {
		return nil
	}
}

func AuthToString[T any](sa *T) string {
	if res, err := json.Marshal(sa); err != nil {
		return fmt.Sprintf("<ERROR: %q>", err.Error())
	} else {
		return string(res)
	}
}

func RegisterProviderConfig(configMapping ConfigRegisterMap) {
	providerConfigMapping = lo.Assign(providerConfigMapping, configMapping)
}

// PluginOwnedKind labels which role a plugin-registered type fills so the
// three Detect call-sites in read.go and the three Prepare*ForDeploy
// branches in client.go can tell plugin-owned templates from
// plugin-owned resources/registrars/provisioner-fields.
//
// Populated at parse time by pkg/stackread via RegisterPluginOwnedType —
// one entry per type each declared plugin's Schema RPC advertises.
type PluginOwnedKind string

const (
	PluginOwnedKindResource         PluginOwnedKind = "resource"
	PluginOwnedKindTemplate         PluginOwnedKind = "template"
	PluginOwnedKindRegistrar        PluginOwnedKind = "registrar"
	PluginOwnedKindStateStore       PluginOwnedKind = "state-store"
	PluginOwnedKindSecretsProvider  PluginOwnedKind = "secrets-provider"
	PluginOwnedKindComputeProcessor PluginOwnedKind = "compute-processor"
	PluginOwnedKindVmDriver         PluginOwnedKind = "vm-driver"
	PluginOwnedKindProvisionerField PluginOwnedKind = "provisioner-field"
)

// PluginOwnedBinding is the record for a single type → plugin
// association captured by RegisterPluginOwnedType.
type PluginOwnedBinding struct {
	Kind       PluginOwnedKind
	PluginName string // name under server.yaml's plugins: block
}

var pluginOwnedTypes = map[string]PluginOwnedBinding{}

// RegisterPluginOwnedType records that the named type is owned by a
// plugin binary (its Schema RPC advertised it). Installs an opaque
// decoder into providerConfigMapping so the generic parse path at
// pkg/api/read.go recognises the type without needing a Go-link-time
// struct. The decoder preserves Config.Config as a map[string]any — the
// plugin receives the raw YAML tree on the provision bus and decodes it
// into its own typed Input shape there.
//
// If the type was already registered via api.RegisterProviderConfig
// (an in-tree type or a plugin's /api/init.go typed registration), the
// typed decoder wins — do NOT overwrite it. This keeps the pre-parse
// strictly ADDITIVE: a plugin that cached /api still gives SC core the
// richer parse-time validation it had, and types SC core has never
// heard of fall through to the opaque path.
//
// pluginName is the key under server.yaml's `plugins:` block (the
// stackread caller passes it through from descriptor.Plugins). The
// A2 ConvertStackConfig path uses it to dial the right plugin.
func RegisterPluginOwnedType(name string, kind PluginOwnedKind, pluginName string) {
	if name == "" {
		return
	}
	pluginOwnedTypes[name] = PluginOwnedBinding{Kind: kind, PluginName: pluginName}
	if _, exists := providerConfigMapping[name]; !exists {
		providerConfigMapping[name] = pluginOwnedConfigDecoder
	}
}

// IsPluginOwnedType reports whether name was recorded by
// RegisterPluginOwnedType and returns the full binding (kind + plugin
// name). Lookups are case sensitive — plugin type names are matched
// byte-for-byte.
func IsPluginOwnedType(name string) (PluginOwnedBinding, bool) {
	b, ok := pluginOwnedTypes[name]
	return b, ok
}

// pluginOwnedConfigDecoder is the ConfigReaderFunc shared by every
// plugin-owned type. It keeps the raw Config.Config map intact so the
// plugin receives the operator's YAML tree verbatim. The plugin side
// decodes into its typed Input via yaml round-trip (same shape the
// Provision RPC already uses).
func pluginOwnedConfigDecoder(cfg *Config) (Config, error) {
	if cfg == nil {
		return Config{}, nil
	}
	return *cfg, nil
}

func RegisterProvisioner(provisionerMapping ProvisionerRegisterMap) {
	provisionerConfigMapping = lo.Assign(provisionerConfigMapping, provisionerMapping)
}

func RegisterProvisionerFieldConfig(mapping ProvisionerFieldConfigRegister) {
	provisionerFieldConfigMapping = lo.Assign(provisionerFieldConfigMapping, mapping)
}

func RegisterCloudSingleImageConverter(mapping CloudSingleImageConfigRegister) {
	cloudSingleImageConverterMapping = lo.Assign(cloudSingleImageConverterMapping, mapping)
}

func RegisterCloudComposeConverter(mapping CloudComposeConfigRegister) {
	cloudComposeConverterMapping = lo.Assign(cloudComposeConverterMapping, mapping)
}

func RegisterCloudStaticSiteConverter(mapping CloudStaticSiteConfigRegister) {
	cloudStaticSiteConverterMapping = lo.Assign(cloudStaticSiteConverterMapping, mapping)
}

func RegisterCloudHelper(mapping CloudHelpersRegisterMap) {
	cloudHelpersConfigMapping = lo.Assign(cloudHelpersConfigMapping, mapping)
}

// GetRegisteredProviderConfigs returns all registered provider configurations
// Used by schema generator to automatically discover all resource types
func GetRegisteredProviderConfigs() ConfigRegisterMap {
	return providerConfigMapping
}

// GetRegisteredProvisionerFieldConfigs returns all registered provisioner field configurations
// Used by schema generator to automatically discover provisioner resource types
func GetRegisteredProvisionerFieldConfigs() ProvisionerFieldConfigRegister {
	return provisionerFieldConfigMapping
}

// GetRegisteredCloudHelpers returns all registered cloud helper configurations
// Used by schema generator to automatically discover cloud helper types
func GetRegisteredCloudHelpers() CloudHelpersRegisterMap {
	return cloudHelpersConfigMapping
}

type CloudHelper interface {
	Run() error
	SetLogger(l logger.Logger)
}

type Provisioner interface {
	ProvisionStack(ctx context.Context, cfg *ConfigFile, stack Stack, params ProvisionParams) error

	SetPublicKey(pubKey string)

	DeployStack(ctx context.Context, cfg *ConfigFile, stack Stack, params DeployParams) error

	DestroyChildStack(ctx context.Context, cfg *ConfigFile, stack Stack, params DestroyParams, preview bool) error

	PreviewStack(ctx context.Context, cfg *ConfigFile, parentStack Stack, params ProvisionParams) (*PreviewResult, error)

	PreviewChildStack(ctx context.Context, cfg *ConfigFile, parentStack Stack, params DeployParams) (*PreviewResult, error)

	OutputsStack(ctx context.Context, cfg *ConfigFile, stack Stack, params StackParams) (*OutputsResult, error)

	CancelStack(ctx context.Context, cfg *ConfigFile, stack Stack, params StackParams) error

	DestroyParentStack(ctx context.Context, cfg *ConfigFile, parentStack Stack, params DestroyParams, preview bool) error

	SetConfigReader(ProvisionerFieldConfigReaderFunc)
}

type ProvisionerOption func(p Provisioner) error

type CloudHelperOption func(c CloudHelper) error

func WithLogger(l logger.Logger) CloudHelperOption {
	return func(c CloudHelper) error {
		c.SetLogger(l)
		return nil
	}
}

func WithFieldConfigReader(f ProvisionerFieldConfigReaderFunc) ProvisionerOption {
	return func(p Provisioner) error {
		p.SetConfigReader(f)
		return nil
	}
}

// WithProviderType defines configuration with specific cloud provider type
type WithProviderType interface {
	ProviderType() string
}

// AuthConfig defines configuration for a single cloud provider
type AuthConfig interface {
	WithProviderType
	CredentialsValue() string
	ProjectIdValue() string
}

// WithDependencyProviders defines configurations where extra cloud providers are required
type WithDependencyProviders interface {
	DependencyProviders() map[string]AuthDescriptor
}

type StateStorageConfig interface {
	AuthConfig
	StorageUrl() string
	IsProvisionEnabled() bool
}

type SecretsProviderConfig interface {
	AuthConfig
	IsProvisionEnabled() bool
	KeyUrl() string
}

type Credentials struct {
	Credentials string `json:"credentials" yaml:"credentials"` // required for proper deserialization
}

type RegistrarConfig interface {
	ProviderType() string
	DnsRecords() []DnsRecord
}

type (
	clientConfigConvertFunc func(cfg *Config) (Config, error)
	clientConfigPrepareFunc func(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error)
)

var clientConfigsPrepareMap = map[string]clientConfigPrepareFunc{
	ClientTypeCloudCompose: func(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
		configCompose, ok := clientDesc.Config.Config.(*StackConfigCompose)
		if !ok {
			return nil, errors.Errorf("client config is not of type *StackConfigCompose")
		}
		return PrepareCloudComposeForDeploy(ctx, stackDir, stackName, tpl, configCompose, clientDesc.ParentStack)
	},
	ClientTypeSingleImage: func(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
		configSingleImage, ok := clientDesc.Config.Config.(*StackConfigSingleImage)
		if !ok {
			return nil, errors.Errorf("client config is not of type *StackConfigSingleImage")
		}
		return PrepareCloudSingleImageForDeploy(ctx, stackDir, stackName, tpl, configSingleImage, clientDesc.ParentStack)
	},
	ClientTypeStatic: func(ctx context.Context, stackDir, stackName string, tpl StackDescriptor, clientDesc StackClientDescriptor) (*StackDescriptor, error) {
		configStatic, ok := clientDesc.Config.Config.(*StackConfigStatic)
		if !ok {
			return nil, errors.Errorf("client config is not of type *StackConfigStatic")
		}
		return PrepareStaticForDeploy(ctx, stackDir, stackName, tpl, configStatic, clientDesc.ParentStack)
	},
}

var clientConfigsConvertMap = map[string]clientConfigConvertFunc{
	ClientTypeStatic: func(cfg *Config) (Config, error) {
		return ConvertConfig(cfg, &StackConfigStatic{})
	},
	ClientTypeSingleImage: func(cfg *Config) (Config, error) {
		return ConvertConfig(cfg, &StackConfigSingleImage{})
	},
	ClientTypeCloudCompose: func(cfg *Config) (Config, error) {
		return ConvertConfig(cfg, &StackConfigCompose{})
	},
}
