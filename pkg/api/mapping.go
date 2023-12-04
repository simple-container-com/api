package api

import (
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

type ConfigReaderFunc func(any) (any, error)

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

func RegisterProviderConfig(configMapping map[string]ConfigReaderFunc) {
	providerConfigMapping = lo.Assign(providerConfigMapping, configMapping)
}
