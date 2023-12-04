package api

const AuthTypeGCPServiceAccount = "gcp-service-account"
const SecretsTypeGCPSecretsManager = "gcp-secrets-manager"
const TemplateTypeGcpCloudrun = "cloudrun"

type GcloudAuthServiceAccountConfig struct {
	Account string `json:"account"`
}

type GcloudSecretsConfig struct {
	Credentials string `json:"credentials"`
}

type GcloudTemplateConfig struct {
	Credentials string `json:"credentials"`
}

func GcloudReadAuthServiceAccountConfig(config any) (any, error) {
	return ConvertDescriptor(config, &GcloudAuthServiceAccountConfig{})
}

func GcloudReadSecretsConfig(config any) (any, error) {
	return ConvertDescriptor(config, &GcloudSecretsConfig{})
}

func GcloudReadTemplateConfig(config any) (any, error) {
	return ConvertDescriptor(config, &GcloudTemplateConfig{})
}
