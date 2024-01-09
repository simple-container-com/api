package gcloud

import "api/pkg/api"

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
)

type AuthServiceAccountConfig struct {
	api.AuthConfig
	Account   string `json:"account" yaml:"account"`
	ProjectId string `json:"projectId" yaml:"projectId"`
}

type SecretsConfig struct {
	api.AuthConfig
	Credentials string `json:"credentials"`
	ProjectId   string `json:"projectId" yaml:"projectId"`
}

func (sa *AuthServiceAccountConfig) CredentialsValue() string {
	return sa.Account
}

func (sa *AuthServiceAccountConfig) ProjectIdValue() string {
	return sa.ProjectId
}

func (sa *SecretsConfig) CredentialsValue() string {
	return sa.Credentials
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
