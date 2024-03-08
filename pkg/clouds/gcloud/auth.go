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

type AuthServiceAccountConfig interface {
	api.AuthConfig
}

type Credentials struct {
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	api.Credentials      `json:",inline" yaml:",inline"`
	BucketName           string `json:"bucketName" yaml:"bucketName"`
	Provision            bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	api.Credentials      `json:",inline" yaml:",inline"`
	KeyRingName          string `json:"keyRingName" yaml:"keyRingName"`
	KeyName              string `json:"keyName" yaml:"keyName"`
	KeyLocation          string `json:"keyLocation" yaml:"keyLocation"`
	KeyRotationPeriod    string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`
	Provision            bool   `json:"provision" yaml:"provision"`
}

func (sa *StateStorageConfig) CredentialsValue() string {
	return sa.Credentials.Credentials // just return serialized gcp account json
}

func (sa *StateStorageConfig) ProjectIdValue() string {
	return sa.ProjectId
}

func (sa *StateStorageConfig) StorageUrl() string {
	return fmt.Sprintf("gs://%s", sa.BucketName)
}

func (sa *StateStorageConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (sa *SecretsProviderConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (sa *Credentials) CredentialsValue() string {
	return sa.Credentials.Credentials // just return serialized gcp account json
}

func (sa *Credentials) ProjectIdValue() string {
	return sa.ProjectId
}

func (sa *SecretsProviderConfig) CredentialsValue() string {
	return sa.Credentials.Credentials // just return serialized gcp account json
}

func (sa *SecretsProviderConfig) ProjectIdValue() string {
	return sa.ProjectId
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
