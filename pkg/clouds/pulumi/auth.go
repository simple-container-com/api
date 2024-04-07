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
	Value string `json:"value" yaml:"value"`
}

func (d *TokenAuthDescriptor) StorageUrl() string {
	return "https://api.pulumi.com"
}

func (d *TokenAuthDescriptor) IsProvisionEnabled() bool {
	return true
}

func (d *TokenAuthDescriptor) CredentialsValue() string {
	return d.Value
}

func (d *TokenAuthDescriptor) ProviderType() string {
	return "pulumi"
}

func (d *TokenAuthDescriptor) ProjectIdValue() string {
	return ""
}

type ProvisionerConfig struct {
	Organization    string                `json:"organization" yaml:"organization"`
	StateStorage    StateStorageConfig    `json:"state-storage" yaml:"state-storage"`
	SecretsProvider SecretsProviderConfig `json:"secrets-provider" yaml:"secrets-provider"`
}

type StateStorageConfig struct {
	Type       string `json:"type" yaml:"type"`
	api.Config `json:",inline" yaml:",inline"`
}

type SecretsProviderConfig struct {
	Type       string `json:"type" yaml:"type"`
	api.Config `json:",inline" yaml:",inline"`
}

func ReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ProvisionerConfig{})
}

func ReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TokenAuthDescriptor{})
}
