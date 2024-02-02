package pulumi

import (
	"api/pkg/clouds/pulumi/gcp"
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

type provisionFunc func(sdkCtx *sdk.Context, input api.ResourceInput) (*api.ResourceOutput, error)

var provisionFuncByType = map[string]provisionFunc{
	gcloud.ResourceTypeBucket: gcp.ProvisionBucket,
}
