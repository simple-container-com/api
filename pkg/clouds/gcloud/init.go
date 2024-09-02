package gcloud

import (
	"github.com/simple-container-com/api/pkg/api"
)

const ProviderType = "gcp"

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		// gcloud
		SecretsTypeGCPSecretsManager: ReadSecretsProviderConfig,
		TemplateTypeGcpCloudrun:      ReadTemplateConfig,
		TemplateTypeStaticWebsite:    ReadTemplateConfig,
		AuthTypeGCPServiceAccount:    ReadAuthServiceAccountConfig,
		TemplateTypeGkeAutopilot:     ReadGkeAutopilotTemplateConfig,
		ResourceTypeGkeAutopilot:     ReadGkeAutopilotResourceConfig,

		// postgres
		ResourceTypePostgresGcpCloudsql: PostgresqlGcpCloudsqlReadConfig,

		// bucket
		ResourceTypeBucket: GcpBucketReadConfig,

		// artifact-registry
		ResourceTypeArtifactRegistry: ArtifactRegistryConfigReadConfig,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeGcpBucket: ReadStateStorageConfig,
		SecretsProviderTypeGcpKms: ReadSecretsProviderConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeGcpCloudrun:  ToCloudRunConfig,
		ResourceTypeGkeAutopilot: ToGkeAutopilotConfig,
	})

	api.RegisterCloudStaticSiteConverter(api.CloudStaticSiteConfigRegister{
		TemplateTypeStaticWebsite: ToStaticSiteConfig,
	})
}
