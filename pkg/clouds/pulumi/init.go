package pulumi

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/gcp"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
)

func init() {
	api.RegisterProviderConfig(api.ConfigRegisterMap{
		ProvisionerTypePulumi: ReadProvisionerConfig,
		AuthTypePulumiToken:   ReadAuthConfig,
	})

	api.RegisterProvisioner(api.ProvisionerRegisterMap{
		ProvisionerTypePulumi: InitPulumiProvisioner,
	})
}

type provisionFunc func(sdkCtx *sdk.Context, input api.ResourceInput, provider params.ProvisionParams) (*api.ResourceOutput, error)

var provisionFuncByType = map[string]provisionFunc{
	gcloud.ResourceTypeBucket: gcp.ProvisionBucket,
}

type provisionerParamsFunc func(ctx *sdk.Context, input params.ProviderInput) (params.ProviderOutput, error)

var providerByType = map[string]provisionerParamsFunc{
	gcloud.ResourceTypeBucket: gcp.ProvisionProvider,
}
