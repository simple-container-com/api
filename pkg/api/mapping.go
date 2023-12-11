package api

import (
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

const MetaDirectoryName = ".sc"

type ConfigReaderFunc func(config *Config) (Config, error)

type ConfigRegisterMap map[string]ConfigReaderFunc

var providerConfigMapping = map[string]ConfigReaderFunc{}

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

func RegisterProviderConfig(configMapping map[string]ConfigReaderFunc) {
	providerConfigMapping = lo.Assign(providerConfigMapping, configMapping)
}

type AuthConfig interface {
	AuthValue() string
}
