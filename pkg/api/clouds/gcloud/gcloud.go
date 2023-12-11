package gcloud

import "api/pkg/api"

const AuthTypeGCPServiceAccount = "gcp-service-account"
const SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
const TemplateTypeGcpCloudrun = "cloudrun"

type GcloudAuthServiceAccountConfig struct {
	api.AuthConfig
	Account string `json:"account"`
}

type GcloudSecretsConfig struct {
	api.AuthConfig
	Credentials string `json:"credentials"`
}

type GcloudTemplateConfig struct {
	Credentials string `json:"credentials"`
}

func (sa *GcloudAuthServiceAccountConfig) AuthValue() string {
	return sa.Account
}

func (sa *GcloudSecretsConfig) AuthValue() string {
	return sa.Credentials
}

func GcloudReadAuthServiceAccountConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcloudAuthServiceAccountConfig{})
}

func GcloudReadSecretsConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcloudSecretsConfig{})
}

func GcloudReadTemplateConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &GcloudTemplateConfig{})
}
