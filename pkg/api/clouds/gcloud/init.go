package gcloud

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// gcloud
		SecretsTypeGCPSecretsManager: GcloudReadSecretsConfig,
		TemplateTypeGcpCloudrun:      GcloudReadTemplateConfig,
		AuthTypeGCPServiceAccount:    GcloudReadAuthServiceAccountConfig,

		// postgres
		ResourceTypePostgresGcpCloudsql: PostgresqlGcpCloudsqlReadConfig,
	})
}
