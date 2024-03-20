package pulumi

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	awsApi "github.com/simple-container-com/api/pkg/clouds/aws"
	gcpApi "github.com/simple-container-com/api/pkg/clouds/gcloud"
	awsImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/aws"
	gcpImpl "github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

type provisionFunc func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error)

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
}

var providerFuncByType = map[string]provisionFunc{
	// gcp
	gcpApi.ProviderType: gcpImpl.ProvisionProvider,
	// aws
	awsApi.ProviderType: awsImpl.ProvisionProvider,
}
