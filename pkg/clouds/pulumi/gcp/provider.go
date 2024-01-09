package gcp

import (
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ProviderInput struct {
	Name        string
	Credentials string
	ProjectId   string
}

func ProvisionProvider(ctx *sdk.Context, params ProviderInput) (sdk.ProviderResource, error) {
	return gcp.NewProvider(ctx, params.Name, &gcp.ProviderArgs{
		Credentials: sdk.String(params.Credentials),
		Project:     sdk.String(params.ProjectId),
	})
}
