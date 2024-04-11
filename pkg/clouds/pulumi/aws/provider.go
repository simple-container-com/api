package aws

import (
	"github.com/pkg/errors"
	sdkAws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func ProvisionProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	authCfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	var pcfg aws.AccountConfig
	if err := api.ConvertAuth(authCfg, &pcfg); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to aws.AccountConfig")
	}

	provider, err := sdkAws.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &sdkAws.ProviderArgs{
		AccessKey: sdk.String(pcfg.AccessKey),
		SecretKey: sdk.String(pcfg.SecretAccessKey),
		Region:    sdk.String(pcfg.Region),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
