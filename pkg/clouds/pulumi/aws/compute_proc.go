package aws

import (
	"fmt"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func RdsPostgresComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	postgresCfg, ok := input.Descriptor.Config.Config.(*aws.PostgresConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgres config for %q", input.Descriptor.Type)
	}
	accountConfig := &aws.AccountConfig{}
	err := api.ConvertAuth(&postgresCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}

	postgresCfg.AccountConfig = *accountConfig
	postgresResName := lo.If(postgresCfg.Name == "", input.Descriptor.Name).Else(postgresCfg.Name)
	postgresName := toRdsPostgresName(postgresResName, input.StackParams.Environment)

	// Create a StackReference to the parent stack
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for rds postgres %q", params.ParentStack.FullReference, postgresName)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s--pg-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}
	postgresEndpointExport := toPostgresInstanceEndpointExport(postgresName)
	resPgEndpoint, err := pApi.GetParentOutput(parentRef, postgresEndpointExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres endpoint from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if resPgEndpoint == "" {
		return nil, errors.Errorf("postgres endpoint is empty for %q (%q)", stack.Name, postgresName)
	}
	postgresUsernameExport := toPostgresInstanceUsernameExport(postgresName)
	resPgUsername, err := pApi.GetParentOutput(parentRef, postgresUsernameExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres username from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if resPgUsername == "" {
		return nil, errors.Errorf("postgres username is empty for %q (%q)", stack.Name, postgresName)
	}
	postgresPasswordExport := toPostgresInstancePasswordExport(postgresName)
	resPgPassword, err := pApi.GetParentOutput(parentRef, postgresPasswordExport, params.ParentStack.FullReference, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres password from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if resPgPassword == "" {
		return nil, errors.Errorf("postgres password is empty for %q (%q)", stack.Name, postgresName)
	}
	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		if err := execEcsTask(ctx, ecsTaskConfig{
			name:    fmt.Sprintf("%s-pg-init", stack.Name),
			account: postgresCfg.AccountConfig,
			params:  params,
			image:   "alpine:latest",
			command: "env",
			env: map[string]string{
				"HELLO": "true",
			},
		}); err != nil {
			return errors.Wrapf(err, "failed to run init task for rds postgres")
		}

		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_%s_URL", postgresResName)), resPgEndpoint,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
			"url": postgresResName,
		})

		return nil
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}

func S3BucketComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucketName := input.ToResName(lo.If(bucketCfg.Name == "", input.Descriptor.Name).Else(bucketCfg.Name))

	// Create a StackReference to the parent stack
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for s3 bucket %q", params.ParentStack.FullReference, bucketName)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s--s3-bucket-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	bucketNameExport := toBucketNameExport(bucketName)
	resBucketName, err := pApi.GetParentOutput(parentRef, bucketNameExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket name from parent stack for %q (%q)", stack.Name, bucketNameExport)
	} else if resBucketName == "" {
		return nil, errors.Errorf("bucket name is empty for %q (%q)", stack.Name, bucketNameExport)
	}
	secretKeyExport := toBucketAccessKeySecretExport(bucketName)
	resAccessKeySecret, err := pApi.GetParentOutput(parentRef, secretKeyExport, params.ParentStack.FullReference, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q (%q)", stack.Name, secretKeyExport)
	} else if resAccessKeySecret == "" {
		return nil, errors.Errorf("bucket access key secret is empty for %q (%q)", stack.Name, secretKeyExport)
	}
	keyIdExport := toBucketAccessKeyIdExport(bucketName)
	resAccessKeyId, err := pApi.GetParentOutput(parentRef, keyIdExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q (%q)", stack.Name, keyIdExport)
	} else if resAccessKeyId == "" {
		return nil, errors.Errorf("bucket access key id is empty for %q (%q)", stack.Name, keyIdExport)
	}
	regionExport := toBucketRegionExport(bucketName)
	resBucketRegion, err := pApi.GetParentOutput(parentRef, regionExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket region from parent stack for %q (%q)", stack.Name, regionExport)
	} else if resBucketRegion == "" {
		return nil, errors.Errorf("bucket region is empty for %q (%q)", stack.Name, regionExport)
	}

	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_REGION", bucketName)), resBucketRegion,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_BUCKET", bucketName)), resBucketName,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_ACCESS_KEY", bucketName)), resAccessKeyId,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_SECRET_KEY", bucketName)), resAccessKeySecret,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_REGION"), resBucketRegion,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_BUCKET"), resBucketName,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_ACCESS_KEY"), resAccessKeyId,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_SECRET_KEY"), resAccessKeySecret,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)

		collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
			"bucket":     resBucketName,
			"region":     resBucketRegion,
			"access-key": resAccessKeyId,
			"secret-key": resAccessKeySecret,
		})

		return nil
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}
