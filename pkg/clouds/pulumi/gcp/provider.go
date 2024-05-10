package gcp

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func InitStateStore(ctx context.Context, authCfg api.AuthConfig) error {
	// hackily set google creds env variable, so that bucket can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
	if err := os.Setenv("GOOGLE_CREDENTIALS", authCfg.CredentialsValue()); err != nil {
		fmt.Println("Failed to set GOOGLE_CREDENTIALS env variable: ", err.Error())
	}
	return nil
}

func Provider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	pcfg, ok := input.Descriptor.Config.Config.(api.AuthConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to api.AuthConfig")
	}

	creds := pcfg.CredentialsValue()
	projectId := pcfg.ProjectIdValue()

	provider, err := gcp.NewProvider(ctx, input.ToResName(input.Descriptor.Name), &gcp.ProviderArgs{
		Credentials: sdk.String(creds),
		Project:     sdk.String(projectId),
	})
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}
