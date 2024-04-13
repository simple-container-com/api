package mongodb

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-mongodbatlas/sdk/v3/go/mongodbatlas"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/mongodb"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	authCfg, ok := input.Descriptor.Config.Config.(*mongodb.AtlasConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to *mongodb.AtlasConfig")
	}

	provider, err := mongodbatlas.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &mongodbatlas.ProviderArgs{
		PrivateKey: sdk.StringPtr(authCfg.PrivateKey),
		PublicKey:  sdk.StringPtr(authCfg.PublicKey),
		Region:     sdk.StringPtr(authCfg.Region),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
