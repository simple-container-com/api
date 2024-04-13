package gcp

import (
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func init() {
	api.RegisterInitStateStore(gcloud.ProviderType, InitStateStore)
	api.RegisterProvider(gcloud.ProviderType, Provider)
	api.RegisterResources(map[string]api.ProvisionFunc{
		gcloud.ResourceTypeBucket:        PrivateBucket,
		gcloud.SecretsProviderTypeGcpKms: KmsKeySecretsProvider,
		gcloud.TemplateTypeGcpCloudrun:   Cloudrun,
		gcloud.TemplateTypeStaticWebsite: StaticWebsite,
	})
	api.RegisterComputeProcessor(map[string]api.ComputeProcessorFunc{
		gcloud.StateStorageTypeGcpBucket: BucketComputeProcessor,
	})
}
