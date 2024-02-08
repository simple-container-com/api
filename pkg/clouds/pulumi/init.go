package pulumi

import (
	"api/pkg/clouds/pulumi/gcp"
	"api/pkg/clouds/pulumi/params"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"api/pkg/api"
	"api/pkg/clouds/gcloud"
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
