package gcloud

import (
	"api/pkg/api"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// gcloud
		SecretsTypeGCPSecretsManager: ReadSecretsConfig,
		TemplateTypeGcpCloudrun:      ReadTemplateConfig,
		AuthTypeGCPServiceAccount:    ReadAuthServiceAccountConfig,

		// postgres
		ResourceTypePostgresGcpCloudsql: PostgresqlGcpCloudsqlReadConfig,
	})
}
