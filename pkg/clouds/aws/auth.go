package aws

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeAWSToken             = "aws-token"
	SecretsTypeAWSSecretsManager = "aws-secrets-manager"

	SecretsProviderTypeAwsKms = "aws-kms"
	StateStorageTypeS3Bucket  = "s3-bucket"
)

type AccountConfig struct {
	Account         string `json:"account" yaml:"account"`
	AccessKey       string `json:"accessKey" yaml:"accessKey"`
	SecretAccessKey string `json:"secretAccessKey" yaml:"secretAccessKey"`
	Region          string `json:"region" yaml:"region"`
	api.Credentials `json:",inline" yaml:",inline"`
}

type SecretsConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
	BucketName    string `json:"bucketName" yaml:"bucketName"`
	Provision     bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	AccountConfig `json:",inline" yaml:",inline"`
	Provision     bool   `json:"provision" yaml:"provision"`
	KeyName       string `json:"keyName" yaml:"keyName"`
}

func (r *AccountConfig) ProviderType() string {
	return ProviderType
}

func (r *AccountConfig) CredentialsValue() string {
	return lo.If(r.Credentials.Credentials == "", api.AuthToString(r)).Else(r.Credentials.Credentials)
}

func (r *AccountConfig) ProjectIdValue() string {
	return r.Account
}

func (sa *StateStorageConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (sa *SecretsProviderConfig) IsProvisionEnabled() bool {
	return sa.Provision
}

func (sa *StateStorageConfig) StorageUrl() string {
	return fmt.Sprintf("s3://%s", sa.BucketName)
}

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AccountConfig{})
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
