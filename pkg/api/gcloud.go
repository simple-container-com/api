package api

const GCPServiceAccountAuthType = "gcp-service-account"
const GcloudAuthType = "gcloud"
const GcloudSecretsType = "gcloud"
const GcloudCloudRunTemplateType = "cloudrun"

type GcloudAuthConfig struct {
	Account string `json:"account"`
}

type GcloudSecretsConfig struct {
	Credentials string `json:"credentials"`
}
