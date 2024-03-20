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
	gcloud.TemplateTypeGcpCloudrun:   gcp.ProvisionCloudrun,

	// aws
	aws.ResourceTypeS3Bucket:      awsImpl.ProvisionBucket,
	aws.SecretsProviderTypeAwsKms: awsImpl.ProvisionKmsKey,
	aws.TemplateTypeEcsFargate:    awsImpl.ProvisionEcsFargate,
}

var providerFuncByType = map[string]provisionFunc{
	// gcp
	gcloud.ProviderType: gcp.ProvisionProvider,
	// aws
	aws.ProviderType: awsImpl.ProvisionProvider,
}
