package aws

import (
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func BucketComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	parentStackName := stack.Client.Stacks[input.DeployParams.StackName].ParentStack

	params.Log.Error(ctx.Context(), "not implemented yet for s3 buckets")

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}
