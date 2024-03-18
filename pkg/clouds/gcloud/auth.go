package gcloud

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"

	StateStorageTypeGcpBucket = "gcp-bucket"
	SecretsProviderTypeGcpKms = "gcp-kms"
)

type ServiceAccountConfig struct {
	ProjectId string `json:"projectId" yaml:"projectId"`
}

type Credentials struct {
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	Credentials `json:",inline" yaml:",inline"`
	BucketName  string `json:"bucketName" yaml:"bucketName"`
	Provision   bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	Credentials       `json:",inline" yaml:",inline"`
	KeyRingName       string `json:"keyRingName" yaml:"keyRingName"`
	KeyName           string `json:"keyName" yaml:"keyName"`
	KeyLocation       string `json:"keyLocation" yaml:"keyLocation"`
	KeyRotationPeriod string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`
	Provision         bool   `json:"provision" yaml:"provision"`
}

func (sa *StateStorageConfig) StorageUrl() string {
	return fmt.Sprintf("gs://%s", sa.BucketName)
}

func (sa *StateStorageConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (r *SecretsProviderConfig) IsProvisionEnabled() bool {
	return r.Provision
}

func (r *Credentials) ProviderType() string {
	return ProviderType
}

func (r *Credentials) ProjectIdValue() string {
	return r.ProjectId
}

func (r *Credentials) CredentialsValue() string {
	return r.Credentials.Credentials // just return serialized gcp account json
}

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &Credentials{})
}

func ReadStateStorageConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &StateStorageConfig{})
}

func ReadSecretsProviderConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsProviderConfig{})
}
