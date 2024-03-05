package pulumi

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypePulumiToken   = "pulumi-token"
	ProvisionerTypePulumi = "pulumi"
)

// TokenAuthDescriptor describes the pulumi token auth schema
type TokenAuthDescriptor struct {
	Value string `json:"value"`
}

type ProvisionerConfig struct {
	Organization    string                `json:"organization" yaml:"organization"`
	StateStorage    StateStorageConfig    `json:"state-storage" yaml:"state-storage"`
	SecretsProvider SecretsProviderConfig `json:"secrets-provider" yaml:"secrets-provider"`
}

type StateStorageConfig struct {
	Type       string `json:"type" yaml:"type"`
	api.Config `json:",inline" yaml:",inline"`
	api.StateStorageConfig
}

type SecretsProviderConfig struct {
	Type       string `json:"type" yaml:"type"`
	api.Config `json:",inline" yaml:",inline"`
	api.SecretsProviderConfig
}

func ReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ProvisionerConfig{})
}

func ReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TokenAuthDescriptor{})
}
