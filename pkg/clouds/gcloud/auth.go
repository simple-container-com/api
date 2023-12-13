package gcloud

import "api/pkg/api"

const (
	AuthTypeGCPServiceAccount    = "gcp-service-account"
	SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
)

type AuthServiceAccountConfig struct {
	api.AuthConfig
	Account string `json:"account"`
}

type SecretsConfig struct {
	api.AuthConfig
	Credentials string `json:"credentials"`
}

func (sa *AuthServiceAccountConfig) AuthValue() string {
	return sa.Account
}

func (sa *SecretsConfig) AuthValue() string {
	return sa.Credentials
}

func ReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &AuthServiceAccountConfig{})
}

func ReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &SecretsConfig{})
}
