package gcloud

import (
	"fmt"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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

type AuthServiceAccountConfig struct {
	api.AuthConfig
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
}

type SecretsConfig struct {
	api.AuthConfig
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	api.StateStorageConfig
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	BucketName           string `json:"bucketName" yaml:"bucketName"`
	Provision            bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	api.SecretsProviderConfig
	api.Credentials      `json:",inline" yaml:",inline"`
	ServiceAccountConfig `json:",inline" yaml:",inline"`
	KeyRingName          string `json:"keyRingName" yaml:"keyRingName"`
	KeyName              string `json:"keyName" yaml:"keyName"`
	KeyLocation          string `json:"keyLocation" yaml:"keyLocation"`
	KeyRotationPeriod    string `json:"keyRotationPeriod" yaml:"keyRotationPeriod"`
	Provision            bool   `json:"provision" yaml:"provision"`
}

func (sa *AuthServiceAccountConfig) ToPulumiProviderArgs() any {
	return &gcp.ProviderArgs{
		Credentials: sdk.String(sa.CredentialsValue()),
		Project:     sdk.String(sa.ProjectId),
	}
}

func (sa *AuthServiceAccountConfig) CredentialsValue() string {
	return sa.Credentials.Credentials
}

func (sa *AuthServiceAccountConfig) ProjectIdValue() string {
	return sa.ProjectId
}

func (sa *SecretsConfig) CredentialsValue() string {
	return sa.Credentials.Credentials // just return serialized gcp account json
}

func (sa *SecretsConfig) ProjectIdValue() string {
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

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AuthServiceAccountConfig{})
}

func ReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsConfig{})
}

func ReadStateStorageConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &StateStorageConfig{})
}

func ReadSecretsProviderConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsProviderConfig{})
}
