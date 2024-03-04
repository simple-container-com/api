package aws

import (
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeAWSToken             = "aws-token"
	SecretsTypeAWSSecretsManager = "aws-secrets-manager"
)

type AuthAccessKeyConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Account         string `json:"account" yaml:"account"`
	AccessKey       string `json:"accessKey" yaml:"accessKey"`
	SecretAccessKey string `json:"secretAccessKey" yaml:"secretAccessKey"`
}

type SecretsConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Account         string `json:"account" yaml:"account"`
	AccessKey       string `json:"accessKey" yaml:"accessKey"`
	SecretAccessKey string `json:"SecretAccessKey" yaml:"SecretAccessKey"`
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

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AuthAccessKeyConfig{})
}

func ReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsConfig{})
}
