package gcp

import (
	"api/pkg/api"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type BucketOutput struct {
	Provider sdk.ProviderResource
}

func ProvisionBucket(ctx *sdk.Context, input api.ResourceInput) (*api.ResourceOutput, error) {
	return &api.ResourceOutput{}, nil
}
