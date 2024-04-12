package pulumi

import (
	"context"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/clouds/cloudflare"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"

	"github.com/simple-container-com/api/pkg/api"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
	gcpApi "github.com/simple-container-com/api/pkg/clouds/gcloud"
	mongodbApi "github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	awsImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	cfImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/cloudflare"
	gcpImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	mongodbImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/mongodb"
)

type (
	provisionFunc        func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error)
	computeProcessorFunc func(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error)
	registrarInitFunc    func(sdkCtx *sdk.Context, desc api.RegistrarDescriptor, params pApi.ProvisionParams) (pApi.Registrar, error)

	initStateStoreFunc func(ctx context.Context, authCfg api.AuthConfig) error
)

var initStateStoreFuncByType = map[string]initStateStoreFunc{
	// gcp
	gcpApi.ProviderType: gcpImpl.InitStateStore,
	// aws
	awsApi.ProviderType: awsImpl.InitStateStore,
}

var providerFuncByType = map[string]provisionFunc{
	// gcp
	gcpApi.ProviderType: gcpImpl.ProvisionProvider,
	// aws
	awsApi.ProviderType: awsImpl.ProvisionProvider,
	// mongodb-atlas
	mongodb.ProviderType: mongodbImpl.ProvisionProvider,
}

var provisionFuncByType = map[string]provisionFunc{
	// gcp
	gcpApi.ResourceTypeBucket:        gcpImpl.ProvisionBucket,
	gcpApi.SecretsProviderTypeGcpKms: gcpImpl.ProvisionKmsKey,
	gcpApi.TemplateTypeGcpCloudrun:   gcpImpl.ProvisionCloudrun,
	gcpApi.TemplateTypeStaticWebsite: gcpImpl.ProvisionStaticWebsite,

	// aws
	awsApi.ResourceTypeS3Bucket:      awsImpl.ProvisionBucket,
	awsApi.SecretsProviderTypeAwsKms: awsImpl.ProvisionKmsKey,
	awsApi.TemplateTypeEcsFargate:    awsImpl.ProvisionEcsFargate,
	awsApi.TemplateTypeStaticWebsite: awsImpl.ProvisionStaticWebsite,

	// mongodb
	mongodbApi.ResourceTypeMongodbAtlas: mongodbImpl.ProvisionCluster,
}

var registrarInitFuncByType = map[string]registrarInitFunc{
	// cloudflare
	cloudflare.RegistrarType: cfImpl.NewCloudflare,

	"": NotConfiguredRegistrar,
}

var computeProcessorFuncByType = map[string]computeProcessorFunc{
	mongodb.ResourceTypeMongodbAtlas: mongodbImpl.MongodbClusterComputeProcessor,
	awsApi.StateStorageTypeS3Bucket:  awsImpl.BucketComputeProcessor,
	gcpApi.ResourceTypeBucket:        gcpImpl.BucketComputeProcessor,
}
