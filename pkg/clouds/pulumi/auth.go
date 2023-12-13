package pulumi

import "api/pkg/api"

const (
	AuthTypePulumiToken   = "pulumi-token"
	ProvisionerTypePulumi = "pulumi"
)

// TokenAuthDescriptor describes the pulumi token auth schema
type TokenAuthDescriptor struct {
	Value string `json:"value"`
}

type ProvisionerConfig struct {
	StateStorage    StateStorageConfig    `json:"state-storage" yaml:"state-storage"`
	SecretsProvider SecretsProviderConfig `json:"secrets-provider" yaml:"secrets-provider"`
}

type StateStorageConfig struct {
	Type         string `json:"type" yaml:"type"`
	Credentials  string `json:"credentials" yaml:"credentials"`
	Organization string `json:"organization" yaml:"organization"`
	Provision    bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	Type         string `json:"type" yaml:"type"`
	Credentials  string `json:"credentials" yaml:"credentials"`
	Organization string `json:"organization" yaml:"organization"`
	Provision    bool   `json:"provision" yaml:"provision"`
}

func ReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ProvisionerConfig{})
}

func ReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TokenAuthDescriptor{})
}
