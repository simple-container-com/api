package pulumi

import "api/pkg/api"

const AuthTypePulumiToken = "pulumi-token"
const ProvisionerTypePulumi = "pulumi"

const StateStorageTypeGcpBucket = "gcp-bucket"
const SecretsProviderTypeGcpKms = "gcp-kms"

// PulumiTokenAuthDescriptor describes the pulumi token auth schema
type PulumiTokenAuthDescriptor struct {
	Value string `json:"value"`
}

type PulumiProvisionerConfig struct {
	StateStorage    PulumiStateStorageConfig    `json:"state-storage" yaml:"state-storage"`
	SecretsProvider PulumiSecretsProviderConfig `json:"secrets-provider" yaml:"secrets-provider"`
}

type PulumiStateStorageConfig struct {
	Type        string `json:"type" yaml:"type"`
	Credentials string `json:"credentials" yaml:"credentials"`
	Provision   bool   `json:"provision" yaml:"provision"`
}

type PulumiSecretsProviderConfig struct {
	Type        string `json:"type" yaml:"type"`
	Credentials string `json:"credentials" yaml:"credentials"`
	Provision   bool   `json:"provision" yaml:"provision"`
}

func PulumiReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PulumiProvisionerConfig{})
}

func PulumiReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &PulumiTokenAuthDescriptor{})
}
