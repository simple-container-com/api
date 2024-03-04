package gcloud

import (
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
)

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
)

type AuthServiceAccountConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	Account         string `json:"account" yaml:"account"`
	ProjectId       string `json:"projectId" yaml:"projectId"`
}

type SecretsConfig struct {
	api.AuthConfig
	api.Credentials `json:",inline" yaml:",inline"`
	ProjectId       string `json:"projectId" yaml:"projectId"`
}

func (sa *AuthServiceAccountConfig) ToPulumiProviderArgs() any {
	return &gcp.ProviderArgs{
		Credentials: sdk.String(sa.Account),
		Project:     sdk.String(sa.ProjectId),
	}
}

func (sa *AuthServiceAccountConfig) CredentialsValue() string {
	return sa.Account
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

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AuthServiceAccountConfig{})
}

func ReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsConfig{})
}
