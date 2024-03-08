package gcp

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/params"
	"os"
)

func ProvisionProvider(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params params.ProvisionParams) (*api.ResourceOutput, error) {
	providerArgs, ok := input.Descriptor.Config.Config.(*gcp.ProviderArgs)
	if !ok {
		return &api.ResourceOutput{}, errors.Errorf("failed to cast config to gcp.ProviderArgs for %q in stack %q", input.Descriptor.Type, stack.Name)
	}

	provider, err := gcp.NewProvider(ctx, input.Descriptor.Name, providerArgs)
	return &api.ResourceOutput{
		Ref: provider,
	}, err
}

func ToPulumiProviderArgs(config api.Config) (any, error) {
	pcfg, ok := config.Config.(gcloud.AuthServiceAccountConfig)
	if !ok {
		return nil, errors.Errorf("failed to cast config to gcloud.AuthServiceAccountConfig")
	}

	creds := pcfg.CredentialsValue()
	projectId := pcfg.ProjectIdValue()
	// hackily set google creds env variable, so that bucket can access it (see github.com/pulumi/pulumi/pkg/v3/authhelpers/gcpauth.go:28)
	if err := os.Setenv("GOOGLE_CREDENTIALS", creds); err != nil {
		fmt.Println("Failed to set GOOGLE_CREDENTIALS env variable: ", err.Error())
	}
	return &gcp.ProviderArgs{
		Credentials: sdk.String(creds),
		Project:     sdk.String(projectId),
	}, nil
}
