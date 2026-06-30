// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

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

	// Export static creds for AWS state-store access ONLY when configured. When
	// empty (OIDC web-identity / instance profile / ambient credentials), leave the
	// environment untouched — otherwise we would blank out the credentials the AWS
	// default chain (e.g. the GitHub OIDC creds the runner already exported) relies on.
	if pcfg.AccessKey != "" {
		if err := os.Setenv("AWS_ACCESS_KEY", pcfg.AccessKey); err != nil {
			fmt.Println("Failed to set AWS_ACCESS_KEY env variable: ", err.Error())
		}
	}
	if pcfg.SecretAccessKey != "" {
		if err := os.Setenv("AWS_SECRET_ACCESS_KEY", pcfg.SecretAccessKey); err != nil {
			fmt.Println("Failed to set AWS_SECRET_ACCESS_KEY env variable: ", err.Error())
		}
	}
	if pcfg.Region != "" {
		if err := os.Setenv("AWS_DEFAULT_REGION", pcfg.Region); err != nil {
			fmt.Println("Failed to set AWS_DEFAULT_REGION env variable: ", err.Error())
		}
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

	providerArgs := &sdkAws.ProviderArgs{
		Region: sdk.String(pcfg.Region),
	}
	// Pin static creds only when configured; otherwise fall back to the AWS default
	// credential chain (OIDC web-identity / instance profile / env). Mirrors the
	// guarded handling already in cloudtrail_security_alerts.go.
	if pcfg.AccessKey != "" {
		providerArgs.AccessKey = sdk.String(pcfg.AccessKey)
	}
	if pcfg.SecretAccessKey != "" {
		providerArgs.SecretKey = sdk.String(pcfg.SecretAccessKey)
	}
	provider, err := sdkAws.NewProvider(ctx, input.ToResName(input.Descriptor.Name), providerArgs)
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
