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
	pcfg, ok := input.Descriptor.Config.Config.(aws.AuthAccessKeyConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to AuthAccessKeyConfig")
	}

	provider, err := sdkAws.NewProvider(ctx, input.Descriptor.Name, &sdkAws.ProviderArgs{
		AccessKey: sdk.String(pcfg.AccessKey),
		SecretKey: sdk.String(pcfg.SecretAccessKey),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
