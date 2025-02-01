package kubernetes

import (
	"fmt"

	"github.com/pkg/errors"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func HelmRabbitmqOperatorComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	rabbitmqInstance := toRabbitmqInstanceName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference
	params.Log.Info(ctx.Context(), "Getting rabbitmq connection %q from parent stack %q", stack.Name, fullParentReference)
	connectionExport := toRabbitmqConnectionParamsExport(rabbitmqInstance)

	connection, err := readObjectFromStack(ctx, fmt.Sprintf("%s-cproc-connection", rabbitmqInstance), fullParentReference, connectionExport, &RabbitmqConnectionParams{}, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal connection config from parent stack")
	}

	appendContextParams := rabbitmqAppendParams{
		stack:           stack,
		collector:       collector,
		input:           input,
		provisionParams: params,
		connection:      connection,
	}
	if params.UseResources[input.Descriptor.Name] {
		if err := appendUsesRabbitmqResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

type rabbitmqAppendParams struct {
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	provisionParams pApi.ProvisionParams
	connection      *RabbitmqConnectionParams
}

func appendUsesRabbitmqResourceContext(ctx *sdk.Context, params rabbitmqAppendParams) error {
	params.collector.AddOutput(sdk.All(params.connection).ApplyT(func(args []any) (any, error) {
		connection := args[0].(*RabbitmqConnectionParams)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("RABBITMQ_USERNAME"), connection.Username,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("RABBITMQ_HOST"), connection.Host,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("RABBITMQ_PORT"), connection.Port,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("RABBITMQ_PASSWORD"), connection.Password,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("RABBITMQ_URI"), connection.ConnectionString,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
			"password": connection.Password,
			"user":     connection.Username,
			"host":     connection.Host,
			"port":     connection.Port,
			"uri":      connection.ConnectionString,
		})

		return nil, nil
	}))

	return nil
}
