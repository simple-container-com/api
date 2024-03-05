package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/samber/lo"

	"gopkg.in/yaml.v3"
)

const MetaDirectoryName = ".sc"

type ConfigReaderFunc func(config *Config) (Config, error)

type ProvisionerInitFunc func(config Config, opts ...ProvisionerOption) (Provisioner, error)

type ConfigRegisterMap map[string]ConfigReaderFunc

type ProvisionerRegisterMap map[string]ProvisionerInitFunc

var providerConfigMapping = ConfigRegisterMap{}

var provisionerConfigMapping = ProvisionerRegisterMap{}

type ProvisionerFieldConfigReadFunc func(config *Config) (Config, error)
type ProvisionerFieldConfigRegister map[string]ProvisionerFieldConfigReadFunc
type ProvisionerFieldConfigReaderFunc func(cType string, c *Config) (Config, error)

var provisionerFieldConfigMapping = ProvisionerFieldConfigRegister{}

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

func RegisterProvisioner(provisionerMapping ProvisionerRegisterMap) {
	provisionerConfigMapping = lo.Assign(provisionerConfigMapping, provisionerMapping)
}

func RegisterProvisionerFieldConfig(mapping ProvisionerFieldConfigRegister) {
	provisionerFieldConfigMapping = lo.Assign(provisionerFieldConfigMapping, mapping)
}

type Provisioner interface {
	ProvisionStack(ctx context.Context, cfg *ConfigFile, pubKey string, stack Stack) error

	SetConfigReader(ProvisionerFieldConfigReaderFunc)
}

type ProvisionerOption func(p Provisioner) error

func WithFieldConfigReader(f ProvisionerFieldConfigReaderFunc) ProvisionerOption {
	return func(p Provisioner) error {
		p.SetConfigReader(f)
		return nil
	}
}

type AuthConfig interface {
	CredentialsValue() string
	ProjectIdValue() string
	ToPulumiProviderArgs() any // deprecated: figure out how to migrate to provisioner-specific implementations
}

type StateStorageConfig interface {
	AuthConfig
	StorageUrl() string
	IsProvisionEnabled() bool
}

type SecretsProviderConfig interface {
	AuthConfig
	IsProvisionEnabled() bool
}

type Credentials struct {
	Credentials string `json:"credentials" yaml:"credentials"` // required for proper deserialization
}
