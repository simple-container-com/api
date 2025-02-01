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

		// redis
		ResourceTypeRedis: RedisReadConfig,

		// bucket
		ResourceTypeBucket: GcpBucketReadConfig,

		// pubsub
		ResourceTypePubSub: GcpPubSubTopicsReadConfig,

		// artifact-registry
		ResourceTypeArtifactRegistry: ArtifactRegistryConfigReadConfig,

		// docker images to re-push to artifact registry
		ResourceTypeRemoteDockerImagePush: DockerRemoteImagePushReadConfig,
	})

	api.RegisterProvisionerFieldConfig(api.ProvisionerFieldConfigRegister{
		StateStorageTypeGcpBucket: ReadStateStorageConfig,
		SecretsProviderTypeGcpKms: ReadSecretsProviderConfig,
	})

	api.RegisterCloudComposeConverter(api.CloudComposeConfigRegister{
		TemplateTypeGcpCloudrun:  ToCloudRunConfig,
		TemplateTypeGkeAutopilot: ToGkeAutopilotConfig,
	})

	api.RegisterCloudStaticSiteConverter(api.CloudStaticSiteConfigRegister{
		TemplateTypeStaticWebsite: ToStaticSiteConfig,
	})
}
