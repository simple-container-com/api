package gcp

import (
	"api/pkg/api"
	"api/pkg/clouds/pulumi/params"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ProvisionProvider(ctx *sdk.Context, input params.ProviderInput) (params.ProviderOutput, error) {
	authCfg, ok := input.Resource.(api.AuthConfig)
	if !ok {
		return params.ProviderOutput{}, errors.Errorf("failed to cast config to AuthConfig for %q", input.Name)
	}

	provider, err := gcp.NewProvider(ctx, input.Name, &gcp.ProviderArgs{
		Credentials: sdk.String(authCfg.CredentialsValue()),
		Project:     sdk.String(authCfg.ProjectIdValue()),
	})
	return params.ProviderOutput{
		Provider: provider,
	}, err
}
