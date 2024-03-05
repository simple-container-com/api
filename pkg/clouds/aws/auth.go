package aws

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	api.AuthConfig
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type SecretsConfig struct {
	api.AuthConfig
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
}

type StateStorageConfig struct {
	api.StateStorageConfig
	api.Credentials  `json:",inline" yaml:",inline"`
	AwsAccountConfig `json:",inline" yaml:",inline"`
	Provision        bool `json:"provision" yaml:"provision"`
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

func (sa *AuthAccessKeyConfig) ToPulumiProviderArgs() any {
	return &aws.ProviderArgs{
		AccessKey: sdk.String(sa.AccessKey),
		SecretKey: sdk.String(sa.SecretAccessKey),
	}
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
