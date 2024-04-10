package aws

import (
	"fmt"
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func BucketComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	parentStackName := stack.Client.Stacks[input.DeployParams.StackName].ParentStack

	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucketName := bucketCfg.Name

	// Create a StackReference to the parent stack
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for s3 bucket", params.ParentStack.RefString)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--s3-bucket-ref", stack.Name, params.ParentStack.StackName), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.RefString).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	resBucketName, err := pApi.GetParentOutput(parentRef, toBucketNameExport(bucketName), params.ParentStack.RefString, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket name from parent stack for %q", stack.Name)
	}
	resAccessKeySecret, err := pApi.GetParentOutput(parentRef, toBucketAccessKeySecretExport(bucketName), params.ParentStack.RefString, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q", stack.Name)
	}
	resAccessKeyId, err := pApi.GetParentOutput(parentRef, toBucketAccessKeyIdExport(bucketName), params.ParentStack.RefString, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q", stack.Name)
	}
	resBucketRegion, err := pApi.GetParentOutput(parentRef, toBucketRegionExport(bucketName), params.ParentStack.RefString, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket region from parent stack for %q", stack.Name)
	}

	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_%s_REGION", bucketCfg.Name)), resBucketRegion)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_%s_BUCKET", bucketCfg.Name)), resBucketName)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_%s_ACCESS_KEY", bucketCfg.Name)), resAccessKeyId)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_%s_SECRET_KEY", bucketCfg.Name)), resAccessKeySecret)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_REGION")), resBucketRegion)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_BUCKET")), resBucketName)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_ACCESS_KEY")), resAccessKeyId)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("S3_SECRET_KEY")), resAccessKeySecret)
		return nil
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}
