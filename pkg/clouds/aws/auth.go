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
}

type AuthAccessKeyConfig struct {
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type SecretsConfig struct {
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
	BucketName       string `json:"bucketName" yaml:"bucketName"`
	Provision        bool   `json:"provision" yaml:"provision"`
}

type SecretsProviderConfig struct {
	api.SecretsProviderConfig
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Provision        bool `json:"provision" yaml:"provision"`
}

func (sa *AuthAccessKeyConfig) CredentialsValue() string {
	return api.AuthToString(sa)
}

func (sa *AuthAccessKeyConfig) ProjectIdValue() string {
	return sa.Account
}

func (sa *SecretsConfig) CredentialsValue() string {
	return sa.SecretAccessKey
}

func (sa *SecretsConfig) ProjectIdValue() string {
	return sa.Account
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
