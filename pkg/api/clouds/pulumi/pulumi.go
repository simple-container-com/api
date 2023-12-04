package pulumi

import "api/pkg/api"

const AuthTypePulumiToken = "pulumi-token"
const ProvisionerTypePulumi = "pulumi"

// PulumiTokenAuthDescriptor describes the pulumi token auth schema
type PulumiTokenAuthDescriptor struct {
	Value string `json:"value"`
}

type PulumiProvisionerConfig struct {
	StateStorage    PulumiStateStorageConfig    `json:"state-storage"`
	SecretsProvider PulumiSecretsProviderConfig `json:"secrets-provider"`
}

type PulumiStateStorageConfig struct {
	Type        string `json:"type" yaml:"type"`
	Credentials string `json:"credentials" yaml:"credentials"`
	Provision   bool   `json:"provision" yaml:"provision"`
}

type PulumiSecretsProviderConfig struct {
	Type        string `json:"type"`
	Credentials string `json:"credentials"`
	Provision   bool   `json:"provision"`
}

func PulumiReadProvisionerConfig(config any) (any, error) {
	return api.ConvertDescriptor(config, &PulumiProvisionerConfig{})
}

func PulumiReadAuthConfig(config any) (any, error) {
	return api.ConvertDescriptor(config, &PulumiTokenAuthDescriptor{})
}
