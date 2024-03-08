package pulumi

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/clouds/aws"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	awsImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

type provisionFunc func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error)

var provisionFuncByType = map[string]provisionFunc{
	// gcp
	gcloud.ResourceTypeBucket:        gcp.ProvisionBucket,
	gcloud.SecretsProviderTypeGcpKms: gcp.ProvisionKmsKey,

	// aws
	aws.ResourceTypeS3Bucket:      awsImpl.ProvisionBucket,
	aws.SecretsProviderTypeAwsKms: awsImpl.ProvisionKmsKey,
}

var providerFuncByType = map[string]provisionFunc{
	// gcp
	gcloud.SecretsProviderTypeGcpKms: gcp.ProvisionProvider,
	gcloud.ResourceTypeBucket:        gcp.ProvisionProvider,

	// aws
	aws.ResourceTypeS3Bucket:      awsImpl.ProvisionProvider,
	aws.SecretsProviderTypeAwsKms: awsImpl.ProvisionProvider,
}

type pulumiProviderArgsFunc func(config api.Config) (any, error)

var pulumiProviderArgsByType = map[string]pulumiProviderArgsFunc{
	// gcp
	gcloud.ResourceTypeBucket:        gcp.ToPulumiProviderArgs,
	gcloud.SecretsProviderTypeGcpKms: gcp.ToPulumiProviderArgs,
	// aws
	aws.ResourceTypeS3Bucket:      awsImpl.ToPulumiProviderArgs,
	aws.SecretsProviderTypeAwsKms: awsImpl.ToPulumiProviderArgs,
}
