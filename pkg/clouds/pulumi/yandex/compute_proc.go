// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/yandex"
	"github.com/simple-container-com/api/pkg/util"
)

// ObjectStorageBucketComputeProcessor injects a parent-provisioned bucket into a
// consuming stack's environment.
//
// The env var names are AWS's on purpose — S3_BUCKET, S3_REGION, S3_ACCESS_KEY,
// S3_SECRET_KEY — because Object Storage is S3-compatible and the whole point of
// picking it is that application code does not change. The one addition is
// S3_ENDPOINT: an AWS SDK pointed at default endpoints will happily talk to
// Amazon instead, so a stack that gets the four AWS vars and not the endpoint
// fails in the most confusing way available (a 403 from the wrong cloud).
func ObjectStorageBucketComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	bucketCfg, ok := input.Descriptor.Config.Config.(*yandex.ObjectStorageBucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucketName := input.ToResName(lo.If(bucketCfg.Name == "", input.Descriptor.Name).Else(bucketCfg.Name))

	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for yandex bucket %q (%q)", params.ParentStack.FullReference, bucketName, suffix)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s%s--yc-bucket-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name, suffix), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	readOutput := func(export string, secret bool) (string, error) {
		val, err := pApi.GetParentOutput(parentRef, export, params.ParentStack.FullReference, secret)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get %q from parent stack for %q", export, stack.Name)
		} else if val == "" {
			return "", errors.Errorf("%q is empty for %q", export, stack.Name)
		}
		return val, nil
	}

	resBucketName, err := readOutput(toBucketNameExport(bucketName), false)
	if err != nil {
		return nil, err
	}
	resBucketRegion, err := readOutput(toBucketRegionExport(bucketName), false)
	if err != nil {
		return nil, err
	}
	resEndpoint, err := readOutput(toBucketEndpointExport(bucketName), false)
	if err != nil {
		return nil, err
	}
	resAccessKeyId, err := readOutput(toBucketAccessKeyIdExport(bucketName), false)
	if err != nil {
		return nil, err
	}
	resAccessKeySecret, err := readOutput(toBucketAccessKeySecretExport(bucketName), true)
	if err != nil {
		return nil, err
	}

	collector.AddOutput(ctx, parentRef.Name.ApplyT(func(refName any) any {
		for name, value := range map[string]string{
			fmt.Sprintf("S3_%s_REGION", bucketName):   resBucketRegion,
			fmt.Sprintf("S3_%s_BUCKET", bucketName):   resBucketName,
			fmt.Sprintf("S3_%s_ENDPOINT", bucketName): resEndpoint,
			"S3_REGION":   resBucketRegion,
			"S3_BUCKET":   resBucketName,
			"S3_ENDPOINT": resEndpoint,
		} {
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(name), value,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		}
		for name, value := range map[string]string{
			fmt.Sprintf("S3_%s_ACCESS_KEY", bucketName): resAccessKeyId,
			fmt.Sprintf("S3_%s_SECRET_KEY", bucketName): resAccessKeySecret,
			"S3_ACCESS_KEY": resAccessKeyId,
			"S3_SECRET_KEY": resAccessKeySecret,
		} {
			collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(name), value,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		}

		collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
			"bucket":     resBucketName,
			"region":     resBucketRegion,
			"endpoint":   resEndpoint,
			"access-key": resAccessKeyId,
			"secret-key": resAccessKeySecret,
		})

		return nil
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}
