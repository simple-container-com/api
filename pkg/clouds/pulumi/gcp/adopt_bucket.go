package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/storage"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptPrivateBucket imports an existing GCS bucket into Pulumi state and creates HMAC keys for S3 compatibility
func AdoptPrivateBucket(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeBucket {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	bucketCfg, ok := input.Descriptor.Config.Config.(*gcloud.GcpBucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	if !bucketCfg.Adopt {
		return nil, errors.Errorf("adopt flag not set for resource %q", input.Descriptor.Name)
	}

	if bucketCfg.GetBucketName() == "" {
		return nil, errors.Errorf("bucketName or name is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	// Use identical naming functions as provisioning to ensure export compatibility
	bucketName := input.ToResName(lo.If(bucketCfg.GetBucketName() == "", input.Descriptor.Name).Else(bucketCfg.GetBucketName()))
	opts := []sdk.ResourceOption{sdk.Provider(params.Provider)}

	params.Log.Info(ctx.Context(), "adopting existing GCS bucket %q and creating S3 interoperability", bucketCfg.GetBucketName())

	// Import existing bucket into Pulumi state
	bucket, err := storage.NewBucket(ctx, bucketName, &storage.BucketArgs{
		Name:     sdk.String(bucketCfg.GetBucketName()),
		Location: sdk.String(bucketCfg.Location),
		// Note: We don't need to specify all the bucket configuration since we're importing
		// Pulumi will read the current state from GCP
	}, append(opts, sdk.Import(sdk.ID(bucketCfg.GetBucketName())))...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to import GCS bucket %q", bucketCfg.GetBucketName())
	}

	// Export bucket name and location (same as provisioning)
	ctx.Export(toBucketNameExport(bucketName), bucket.Name)
	ctx.Export(toBucketLocationExport(bucketName), bucket.Location)

	params.Log.Info(ctx.Context(), "creating service account for adopted bucket %q", bucketCfg.GetBucketName())

	// Create a service account for S3-compatible access (same as provisioning)
	serviceAccountName := fmt.Sprintf("%s-bucket-sa", bucketName)
	sa, err := serviceaccount.NewAccount(ctx, serviceAccountName, &serviceaccount.AccountArgs{
		AccountId:   sdk.String(serviceAccountName),
		DisplayName: sdk.String(fmt.Sprintf("Service account for bucket %s", bucketCfg.GetBucketName())),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create service account for bucket %q", bucketCfg.GetBucketName())
	}

	// Grant the service account access to the bucket (same as provisioning)
	_, err = storage.NewBucketIAMMember(ctx, fmt.Sprintf("%s-bucket-iam", bucketName), &storage.BucketIAMMemberArgs{
		Bucket: bucket.Name,
		Role:   sdk.String("roles/storage.objectAdmin"),
		Member: sdk.Sprintf("serviceAccount:%s", sa.Email),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to grant bucket access to service account for %q", bucketCfg.GetBucketName())
	}

	params.Log.Info(ctx.Context(), "creating HMAC key for adopted bucket %q", bucketCfg.GetBucketName())

	// Create HMAC key for S3-compatible access (same as provisioning)
	hmacKeyName := fmt.Sprintf("%s-hmac-key", bucketName)
	hmacKey, err := storage.NewHmacKey(ctx, hmacKeyName, &storage.HmacKeyArgs{
		ServiceAccountEmail: sa.Email,
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create HMAC key for bucket %q", bucketCfg.GetBucketName())
	}

	// Export HMAC credentials (Access Key ID and Secret Key) - same as provisioning
	ctx.Export(toBucketHmacAccessKeyIdExport(bucketName), hmacKey.AccessId)
	ctx.Export(toBucketHmacSecretKeyExport(bucketName), sdk.ToSecret(hmacKey.Secret))

	params.Log.Info(ctx.Context(), "successfully adopted GCS bucket %q with S3 interoperability", bucketCfg.GetBucketName())

	return &api.ResourceOutput{Ref: bucket}, nil
}
