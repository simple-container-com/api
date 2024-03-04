package aws

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionProvider(ctx *sdk.Context, input params.ProviderInput) (params.ProviderOutput, error) {
	authCfg, ok := input.Resource.(api.AuthConfig)
	if !ok {
		return params.ProviderOutput{}, errors.Errorf("failed to cast config to AuthConfig for %q", input.Name)
	}

	provider, err := aws.NewProvider(ctx, input.Name, &aws.ProviderArgs{
		AccessKey: sdk.String(authCfg.CredentialsValue()),
		SecretKey: sdk.String(authCfg.ProjectIdValue()),
	})
	return params.ProviderOutput{
		Provider: provider,
	}, err
}
