package api

const AuthTypeGCPServiceAccount = "gcp-service-account"
const AuthTypeGcloud = "gcloud"
const SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
const TemplateTypeGcpCloudrun = "cloudrun"

type GcloudAuthConfig struct {
	Account string `json:"account"`
}

type GcloudSecretsConfig struct {
	Credentials string `json:"credentials"`
}

type GcloudTemplateConfig struct {
	Credentials string `json:"credentials"`
}

func GcloudReadSecretsConfig(config any) any {
	return &GcloudSecretsConfig{}
}

func GcloudReadTemplateConfig(config any) any {
	return &GcloudTemplateConfig{}
}
