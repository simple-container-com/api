package gcp

import (
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func ProvisionBucket(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeBucket {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	bucketCfg, ok := input.Descriptor.Config.Config.(*gcloud.GcpBucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucket, err := storage.NewBucket(ctx, bucketCfg.Name, &storage.BucketArgs{
		Name:     sdk.String(bucketCfg.Name),
		Location: sdk.String(bucketCfg.Location),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket %q", bucketCfg.Name)
	}

	return &api.ResourceOutput{Ref: bucket}, nil
}
