package gcp

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionProvider(ctx *sdk.Context, input params.ProviderInput) (params.ProviderOutput, error) {
	authCfg, ok := input.Resource.(api.AuthConfig)
	if !ok {
		return params.ProviderOutput{}, errors.Errorf("failed to cast config to AuthConfig for %q", input.Name)
	}

	providerArgs := authCfg.ToPulumiProviderArgs().(*gcp.ProviderArgs)

	provider, err := gcp.NewProvider(ctx, input.Name, providerArgs)
	return params.ProviderOutput{
		Provider: provider,
	}, err
}
