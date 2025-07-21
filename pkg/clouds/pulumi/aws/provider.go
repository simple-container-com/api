package aws

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"

	sdkAws "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func InitStateStore(ctx context.Context, stateStoreCfg api.StateStorageConfig, log logger.Logger) error {
	var pcfg aws.AccountConfig

	log.Info(ctx, "Initializing aws statestore...")

	if err := api.ConvertAuth(stateStoreCfg, &pcfg); err != nil {
		return errors.Wrapf(err, "failed to convert auth config to aws.AccountConfig")
	}

	// hackily set aws creds env variable, so that we can access AWS state storage
	if err := os.Setenv("AWS_ACCESS_KEY", pcfg.AccessKey); err != nil {
		fmt.Println("Failed to set AWS_ACCESS_KEY env variable: ", err.Error())
	}
	if err := os.Setenv("AWS_SECRET_ACCESS_KEY", pcfg.SecretAccessKey); err != nil {
		fmt.Println("Failed to set AWS_SECRET_ACCESS_KEY env variable: ", err.Error())
	}
	if err := os.Setenv("AWS_DEFAULT_REGION", pcfg.Region); err != nil {
		fmt.Println("Failed to set AWS_DEFAULT_REGION env variable: ", err.Error())
	}
	return nil
}

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
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
