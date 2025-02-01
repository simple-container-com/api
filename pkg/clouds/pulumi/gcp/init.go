package gcp

import (
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterInitStateStore(gcloud.ProviderType, InitStateStore)
	api.RegisterProvider(gcloud.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		gcloud.ResourceTypeBucket:                PrivateBucket,
		gcloud.ResourceTypeRemoteDockerImagePush: RemoteImagePush,
		gcloud.ResourceTypePubSub:                PubSubTopics,
		gcloud.ResourceTypePostgresGcpCloudsql:   Postgres,
		gcloud.ResourceTypeRedis:                 Redis,
		gcloud.ResourceTypeGkeAutopilot:          GkeAutopilot,
		gcloud.ResourceTypeArtifactRegistry:      ArtifactRegistry,
		gcloud.SecretsProviderTypeGcpKms:         KmsKeySecretsProvider,
		gcloud.TemplateTypeGcpCloudrun:           Cloudrun,
		gcloud.TemplateTypeStaticWebsite:         StaticWebsite,
		gcloud.TemplateTypeGkeAutopilot:          GkeAutopilotStack,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		gcloud.ResourceTypeBucket:              BucketComputeProcessor,
		gcloud.ResourceTypeGkeAutopilot:        GkeAutopilotComputeProcessor,
		gcloud.ResourceTypePostgresGcpCloudsql: PostgresComputeProcessor,
		gcloud.ResourceTypeRedis:               RedisComputeProcessor,
		gcloud.ResourceTypePubSub:              PubSubTopicsProcessor,
	})
}
