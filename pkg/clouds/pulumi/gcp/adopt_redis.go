package gcp

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/redis"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

// AdoptRedis imports an existing Redis Memorystore instance into Pulumi state without modifying it
func AdoptRedis(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeRedis {
		return nil, errors.Errorf("unsupported redis type %q", input.Descriptor.Type)
	}

	redisCfg, ok := input.Descriptor.Config.Config.(*gcloud.RedisConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert redis config for %q", input.Descriptor.Type)
	}

	if !redisCfg.Adopt {
		return nil, errors.Errorf("adopt flag not set for resource %q", input.Descriptor.Name)
	}

	if redisCfg.InstanceId == "" {
		return nil, errors.Errorf("instanceId is required when adopt=true for resource %q", input.Descriptor.Name)
	}

	// Use identical naming functions as provisioning to ensure export compatibility
	redisName := toRedisName(input, input.Descriptor.Name)

	params.Log.Info(ctx.Context(), "adopting existing Redis Memorystore instance %q", redisCfg.InstanceId)

	// Import existing Redis instance into Pulumi state
	// The instance resource ID in GCP is: projects/{project}/locations/{location}/instances/{instance}
	// For Redis, we need to construct the full resource path
	instanceResourceId := fmt.Sprintf("projects/%s/locations/%s/instances/%s",
		redisCfg.ProjectId,
		*redisCfg.Region, // Redis requires region
		redisCfg.InstanceId)

	opts := []sdk.ResourceOption{
		sdk.Provider(params.Provider),
		// Import the existing instance without creating or modifying it
		sdk.Import(sdk.ID(instanceResourceId)),
	}

	redisInstance, err := redis.NewInstance(ctx, redisName, &redis.InstanceArgs{
		Name: sdk.String(redisCfg.InstanceId),
		// Note: We don't need to specify all the instance configuration since we're importing
		// Pulumi will read the current state from GCP
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to import Redis Memorystore instance %q", redisCfg.InstanceId)
	}

	// Export the same keys as the provisioning function to ensure compute processor compatibility
	ctx.Export(toRedisHostExport(redisName), redisInstance.Host)
	ctx.Export(toRedisPortExport(redisName), sdk.Sprintf("%d", redisInstance.Port))

	params.Log.Info(ctx.Context(), "successfully adopted Redis Memorystore instance %q", redisCfg.InstanceId)

	return &api.ResourceOutput{Ref: redisInstance}, nil
}
