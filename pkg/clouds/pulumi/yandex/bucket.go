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
	sdkYandex "github.com/simple-container-com/pulumi-yandex/sdk/go/yandex"
)

// ObjectStorageBucketOutput is what the parent stack exports for a bucket.
type ObjectStorageBucketOutput struct {
	Bucket         *sdkYandex.StorageBucket
	ServiceAccount *sdkYandex.IamServiceAccount
	AccessKey      *sdkYandex.IamServiceAccountStaticAccessKey
	RoleBinding    *sdkYandex.ResourcemanagerFolderIamMember
}

// ObjectStorageBucket provisions a Yandex Object Storage bucket together with a
// dedicated service account and the static key pair a consumer signs requests
// with.
//
// The shape is the GCP one, not the AWS one: YC has no IAM *user*, so the
// identity is a service account and the credential is a static access key issued
// against it (IamServiceAccountStaticAccessKey), exactly like GCP's HMAC keys.
// Authorization is a folder-level role binding rather than a bucket policy —
// ResourcemanagerFolderIamMember is additive (unlike ...FolderIamPolicy, which is
// authoritative and would wipe every other binding in the folder; never use that
// one here).
func ObjectStorageBucket(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	bucketCfg, ok := input.Descriptor.Config.Config.(*yandex.ObjectStorageBucket)
	if !ok {
		return nil, errors.Errorf("failed to cast config to yandex.ObjectStorageBucket for %q", input.Descriptor.Type)
	}
	if params.Provider == nil {
		return nil, errors.Errorf("provider must not be nil for bucket %q in stack %q", input.Descriptor.Name, stack.Name)
	}
	// Rehydrate the account config out of `credentials: "${auth:yc}"`, exactly as
	// Provider and ServerlessContainer do. Without this a bucket declared with
	// nothing but a credentials reference — the way every other resource in the
	// fleet is declared — fails with "folderId must be set", because SC resolves
	// `${auth:...}` into the opaque Credentials blob and never into the sibling
	// fields. Live-caught on the first YC smoke provision, 2026-09-26.
	if err := api.ConvertAuth(bucketCfg, &bucketCfg.AccountConfig); err != nil {
		return nil, errors.Wrapf(err, "failed to convert auth config to yandex.AccountConfig for bucket %q", input.Descriptor.Name)
	}
	if bucketCfg.FolderID == "" {
		return nil, errors.Errorf("folderId must be set for bucket %q in stack %q", input.Descriptor.Name, stack.Name)
	}

	bucketName := input.ToResName(lo.If(bucketCfg.Name == "", input.Descriptor.Name).Else(bucketCfg.Name))
	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	params.Log.Info(ctx.Context(), "configure yandex object storage bucket %q for stack %q", bucketName, stack.Name)

	sa, err := sdkYandex.NewIamServiceAccount(ctx, fmt.Sprintf("%s-sa", bucketName), &sdkYandex.IamServiceAccountArgs{
		Name:        sdk.String(fmt.Sprintf("%s-sa", bucketName)),
		FolderId:    sdk.String(bucketCfg.FolderID),
		Description: sdk.String(fmt.Sprintf("service account for bucket %s (stack %s)", bucketName, stack.Name)),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create service account for bucket %q", bucketName)
	}

	// The role binding must exist before the bucket: creating a bucket is itself a
	// storage.editor operation when the provider authenticates as this account, and
	// YC's IAM is eventually consistent enough that ordering matters.
	roleBinding, err := sdkYandex.NewResourcemanagerFolderIamMember(ctx, fmt.Sprintf("%s-role", bucketName), &sdkYandex.ResourcemanagerFolderIamMemberArgs{
		FolderId: sdk.String(bucketCfg.FolderID),
		Role:     sdk.String(bucketCfg.EffectiveRole()),
		Member:   sa.ID().ToStringOutput().ApplyT(func(id string) string { return fmt.Sprintf("serviceAccount:%s", id) }).(sdk.StringOutput),
	}, append(opts, sdk.DependsOn([]sdk.Resource{sa}))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to bind role %q for bucket %q", bucketCfg.EffectiveRole(), bucketName)
	}

	accessKey, err := sdkYandex.NewIamServiceAccountStaticAccessKey(ctx, fmt.Sprintf("%s-key", bucketName), &sdkYandex.IamServiceAccountStaticAccessKeyArgs{
		ServiceAccountId: sa.ID().ToStringOutput(),
		Description:      sdk.String(fmt.Sprintf("static key for bucket %s", bucketName)),
	}, append(opts, sdk.DependsOn([]sdk.Resource{roleBinding}))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create static access key for bucket %q", bucketName)
	}

	bucketArgs := &sdkYandex.StorageBucketArgs{
		Bucket:       sdk.String(bucketName),
		FolderId:     sdk.String(bucketCfg.FolderID),
		ForceDestroy: sdk.Bool(bucketCfg.ForceDestroy),
	}
	if bucketCfg.MaxSize > 0 {
		bucketArgs.MaxSize = sdk.Int(bucketCfg.MaxSize)
	}
	bucket, err := sdkYandex.NewStorageBucket(ctx, bucketName, bucketArgs, append(opts, sdk.DependsOn([]sdk.Resource{roleBinding}))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create bucket %q", bucketName)
	}

	ctx.Export(toBucketNameExport(bucketName), bucket.Bucket)
	ctx.Export(toBucketRegionExport(bucketName), sdk.String(bucketCfg.EffectiveRegion()))
	// The endpoint has no AWS analogue — the AWS compute processor exports none,
	// because an AWS SDK's default resolver already knows the host. For YC the
	// endpoint IS what makes an otherwise-unmodified S3 client work, so it ships
	// alongside the credentials.
	ctx.Export(toBucketEndpointExport(bucketName), sdk.String(yandex.DefaultStorageEndpoint))
	ctx.Export(toBucketAccessKeyIdExport(bucketName), accessKey.AccessKey)
	ctx.Export(toBucketAccessKeySecretExport(bucketName), sdk.ToSecret(accessKey.SecretKey))

	return &api.ResourceOutput{
		Ref: &ObjectStorageBucketOutput{
			Bucket:         bucket,
			ServiceAccount: sa,
			AccessKey:      accessKey,
			RoleBinding:    roleBinding,
		},
	}, nil
}

func toBucketNameExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-name", bucketName)
}

func toBucketRegionExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-region", bucketName)
}

func toBucketEndpointExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-endpoint", bucketName)
}

func toBucketAccessKeyIdExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-name", bucketName)
}

func toBucketAccessKeySecretExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-secret", bucketName)
}
