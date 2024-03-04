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

	secretsProviderOutput *SecretsProviderOutput
}

type StateStorageConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Type            string `json:"type" yaml:"type"`
	ProjectId       string `json:"projectId" yaml:"projectId"`
	BucketName      string `json:"bucketName" yaml:"bucketName"`
	Provision       bool   `json:"provision" yaml:"provision"`
}

func (r *StateStorageConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *StateStorageConfig) ProjectIdValue() string {
	return r.ProjectId
}

type SecretsProviderConfig struct {
	api.AuthConfig
	api.Credentials   `json:",inline" yaml:",inline"`
	Type              string `json:"type" yaml:"type"`
	ProjectId         string `json:"projectId" yaml:"projectId"`
	KeyName           string `json:"keyName" yaml:"keyName"`
	KeyLocation       string `json:"keyLocation" yaml:"keyLocation"`
	KeyRotationPeriod string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`
	Provision         bool   `json:"provision" yaml:"provision"`
}

func (r *SecretsProviderConfig) CredentialsValue() string {
	return r.Credentials.Credentials
}

func (r *SecretsProviderConfig) ProjectIdValue() string {
	return r.ProjectId
}

func ReadProvisionerConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ProvisionerConfig{})
}

func ReadAuthConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &TokenAuthDescriptor{})
}
