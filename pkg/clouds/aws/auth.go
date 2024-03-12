package aws

import (
	"fmt"

	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeAWSToken             = "aws-token"
	SecretsTypeAWSSecretsManager = "aws-secrets-manager"

	SecretsProviderTypeAwsKms = "aws-kms"
	StateStorageTypeS3Bucket  = "s3-bucket"
)

type AwsAccountConfig struct {
	Account         string `json:"account" yaml:"account"`
	AccessKey       string `json:"accessKey" yaml:"accessKey"`
	SecretAccessKey string `json:"secretAccessKey" yaml:"secretAccessKey"`
	api.Credentials `json:",inline" yaml:",inline"`
}

type AuthAccessKeyConfig struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type SecretsConfig struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
	BucketName       string `json:"bucketName" yaml:"bucketName"`
	Provision        bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Provision        bool `json:"provision" yaml:"provision"`
}

func (r *AwsAccountConfig) ProviderType() string {
	return ProviderType
}

func (r *AwsAccountConfig) CredentialsValue() string {
	return api.AuthToString(r)
}

func (r *AwsAccountConfig) ProjectIdValue() string {
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
	return api.ConvertConfig(config, &AuthAccessKeyConfig{})
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
