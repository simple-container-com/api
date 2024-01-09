package pulumi

import (
	"api/pkg/api"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/kms"
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

	kmsKey *kms.CryptoKey
}

type StateStorageConfig struct {
	Type        string `json:"type" yaml:"type"`
	Credentials string `json:"credentials" yaml:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
	BucketName  string `json:"bucketName" yaml:"bucketName"`
	Provision   bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	Type              string `json:"type" yaml:"type"`
	Credentials       string `json:"credentials" yaml:"credentials"`
	ProjectId         string `json:"projectId" yaml:"projectId"`
	KeyName           string `json:"keyName" yaml:"keyName"`
	KeyLocation       string `json:"keyLocation" yaml:"keyLocation"`
	KeyRotationPeriod string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`
	Provision         bool   `json:"provision" yaml:"provision"`
}

func ReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ProvisionerConfig{})
}

func ReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TokenAuthDescriptor{})
}
