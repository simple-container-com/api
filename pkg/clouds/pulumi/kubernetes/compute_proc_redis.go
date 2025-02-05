package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func HelmRedisOperatorComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	redisInstance := toRedisInstanceName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference

	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "Getting redis connection %q from parent stack %q (%q)", stack.Name, fullParentReference, suffix)
	connectionExport := toRedisConnectionParamsExport(redisInstance)

	connection, err := readObjectFromStack(ctx, fmt.Sprintf("%s%s-cproc-connection", redisInstance, suffix), fullParentReference, connectionExport, &RedisConnectionParams{}, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal connection config from parent stack")
	}

	appendContextParams := redisAppendParams{
		stack:           stack,
		collector:       collector,
		input:           input,
		provisionParams: params,
		connection:      connection,
	}
	if params.ParentStack.UsesResource {
		if err := appendUsesRedisResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	} else {
		params.Log.Warn(ctx.Context(), "redis %q only supports `uses`, but it wasn't explicitly declared as being used", redisInstance)
		return nil, errors.Errorf("redis %q only supports `uses`, but it wasn't explicitly declared as being used", redisInstance)
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

type redisAppendParams struct {
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	provisionParams pApi.ProvisionParams
	connection      *RedisConnectionParams
}

func appendUsesRedisResourceContext(ctx *sdk.Context, params redisAppendParams) error {
	params.collector.AddOutput(ctx, sdk.All(params.connection).ApplyT(func(args []any) (any, error) {
		connection := args[0].(*RedisConnectionParams)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("REDIS_HOST"), connection.Host,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("REDIS_PORT"), connection.Port,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
			"host": connection.Host,
			"port": connection.Port,
		})

		return nil, nil
	}))

	return nil
}
