package pulumi

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

type provisionFunc func(sdkCtx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error)

var provisionFuncByType = map[string]provisionFunc{
	gcloud.ResourceTypeBucket:        gcp.ProvisionBucket,
	gcloud.SecretsProviderTypeGcpKms: gcp.ProvisionKmsKey,
}

type provisionerParamsFunc func(ctx *sdk.Context, input params.ProviderInput) (params.ProviderOutput, error)

var providerByType = map[string]provisionerParamsFunc{
	gcloud.ResourceTypeBucket: gcp.ProvisionProvider,
}
