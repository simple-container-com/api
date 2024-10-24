package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/redis"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func Redis(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.ResourceTypeRedis {
		return nil, errors.Errorf("unsupported redis type %q", input.Descriptor.Type)
	}

	redisCfg, ok := input.Descriptor.Config.Config.(*gcloud.RedisConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert redis config for %q", input.Descriptor.Type)
	}

	redisServiceName := fmt.Sprintf("projects/%s/services/redis.googleapis.com", redisCfg.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, redisServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", redisServiceName)
	}

	redisName := toRedisName(input, input.Descriptor.Name)
	redisInstance, err := redis.NewInstance(ctx, redisName, &redis.InstanceArgs{
		MemorySizeGb: sdk.Int(redisCfg.MemorySizeGb),
		RedisConfigs: sdk.ToStringMap(redisCfg.RedisConfig),
		Region:       sdk.StringPtrFromPtr(lo.If(redisCfg.Region != nil, redisCfg.Region).Else(nil)),
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision redis instance %q", redisName)
	}

	ctx.Export(toRedisPortExport(redisName), redisInstance.Port)
	ctx.Export(toRedisHostExport(redisName), redisInstance.Host)

	return &api.ResourceOutput{Ref: redisInstance}, nil
}

func RedisComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	redisName := toRedisName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference
	params.Log.Info(ctx.Context(), "Getting redis host for %q from parent stack %q", stack.Name, fullParentReference)
	redisHostExport := toRedisHostExport(redisName)
	redisHost, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-cproc-host", redisName), fullParentReference, redisHostExport, true)
	if err != nil || redisHost == "" {
		return nil, errors.Wrapf(err, "failed to get redis host from parent stack for %q", redisName)
	}
	params.Log.Info(ctx.Context(), "Getting redis port for %q from parent stack %q", stack.Name, fullParentReference)
	redisPortExport := toRedisPortExport(redisName)
	redisPort, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-cproc-port", redisName), fullParentReference, redisPortExport, true)
	if err != nil || redisPort == "" {
		return nil, errors.Wrapf(err, "failed to get redis port from parent stack for %q", redisName)
	}

	if !params.UseResources[input.Descriptor.Name] {
		params.Log.Warn(ctx.Context(), "redis %q only supports `uses`, but it wasn't explicitly declared as being used", redisName)
		return nil, nil
	}

	if params.UseResources[input.Descriptor.Name] {
		params.Log.Info(ctx.Context(), "Adding REDIS_HOST env variable for stack %q from %q", stack.Name, fullParentReference)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("REDIS_HOST"), redisHost,
			input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)
		params.Log.Info(ctx.Context(), "Adding REDIS_PORT env variable for stack %q from %q", stack.Name, fullParentReference)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("REDIS_PORT"), redisPort,
			input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

		collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
			"host": redisHost,
			"port": redisPort,
		})
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

func toRedisPortExport(resName string) string {
	return fmt.Sprintf("%s-port", resName)
}

func toRedisHostExport(resName string) string {
	return fmt.Sprintf("%s-host", resName)
}

func toRedisName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}
