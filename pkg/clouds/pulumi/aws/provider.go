package aws

import (
	"github.com/pkg/errors"
	sdkAws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
)

func ProvisionProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error) {
	providerArgs, ok := input.Descriptor.Config.Config.(*sdkAws.ProviderArgs)
	if !ok {
		return &api.ResourceOutput{}, errors.Errorf("failed to cast config to gcp.ProviderArgs for %q in stack %q", input.Descriptor.Type, stack.Name)
	}

	provider, err := sdkAws.NewProvider(ctx, input.Descriptor.Name, providerArgs)
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}

func ToPulumiProviderArgs(config api.Config) (any, error) {
	pcfg, ok := config.Config.(aws.AuthAccessKeyConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to AuthAccessKeyConfig")
	}
	return &sdkAws.ProviderArgs{
		AccessKey: sdk.String(pcfg.AccessKey),
		SecretKey: sdk.String(pcfg.SecretAccessKey),
	}, nil
}
