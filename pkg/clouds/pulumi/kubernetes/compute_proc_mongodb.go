package kubernetes

import (
	"fmt"
	"github.com/samber/lo"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/mongodb"
	"github.com/simple-container-com/api/pkg/util"
)

func HelmMongodbOperatorComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	mongoInstance := toMongodbInstanceName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference
	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "Getting mongodb connection %q from parent stack %q (%q)", stack.Name, fullParentReference, suffix)
	connectionExport := toMongodbConnectionParamsExport(mongoInstance)

	connection, err := readObjectFromStack(ctx, fmt.Sprintf("%s%s-cproc-connection", mongoInstance, suffix), fullParentReference, connectionExport, &MongodbConnectionParams{}, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal connection config from parent stack")
	}

	appendContextParams := mongodbAppendParams{
		instanceName:    mongoInstance,
		stack:           stack,
		collector:       collector,
		input:           input,
		provisionParams: params,
		connection:      connection,
		suffix:          suffix,
	}
	if params.ParentStack.UsesResource {
		if err := appendUsesMongodbResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	} else {
		params.Log.Warn(ctx.Context(), "mongodb %q only supports `uses`, but it wasn't explicitly declared as being used", mongoInstance)
		return nil, errors.Errorf("mongodb %q only supports `uses`, but it wasn't explicitly declared as being used", mongoInstance)
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

type mongodbAppendParams struct {
	instanceName    string
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	provisionParams pApi.ProvisionParams
	connection      *MongodbConnectionParams
	suffix          string
}

func appendUsesMongodbResourceContext(ctx *sdk.Context, params mongodbAppendParams) error {
	// set both dbname and username to stack name
	dbName := params.stack.Name
	userName := params.stack.Name

	password, err := createMongodbUserForDatabase(ctx, userName, dbName, params)
	if err != nil {
		return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
	}

	params.collector.AddOutput(sdk.All(params.connection, password.Result).ApplyT(func(args []any) (any, error) {
		rootConnection := args[0].(*MongodbConnectionParams)
		servicePassword := args[1].(string)
		connection := &MongodbConnectionParams{
			InstanceName: rootConnection.InstanceName,
			Host:         rootConnection.Host,
			Port:         rootConnection.Port,
			Password:     servicePassword,
			Username:     userName,
			Database:     dbName,
		}

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_USER"), connection.Username,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_HOST"), connection.Host,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_PORT"), connection.Port,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_PASSWORD"), connection.Password,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_URI"), connection.ConnectionString(),
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		// oplog uri is necessary for apps that would like to read mongo's oplog
		oplogMongoUri := mongodb.AppendUserPasswordAndDBToMongoUri(connection.ConnectionString(), connection.Username, connection.Password, "local")

		params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
			"password": connection.Password,
			"user":     connection.Username,
			"host":     connection.Host,
			"port":     connection.Port,
			"uri":      connection.ConnectionString(),
			"dbName":   dbName,
			"oplogUri": oplogMongoUri,
		})

		return nil, nil
	}))

	return nil
}

func createMongodbUserForDatabase(ctx *sdk.Context, userName, dbName string, params mongodbAppendParams) (*random.RandomPassword, error) {
	ctx.Export(fmt.Sprintf("%s-%s%s-username", userName, params.instanceName, params.suffix), sdk.String(userName))
	passwordName := fmt.Sprintf("%s-%s%s-password", userName, params.instanceName, params.suffix)
	password, err := random.NewRandomPassword(ctx, passwordName, &random.RandomPasswordArgs{
		Length:  sdk.Int(20),
		Special: sdk.Bool(false),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate random password for mongodb for user %q", userName)
	}
	ctx.Export(passwordName, password.Result)

	namespace := params.input.StackParams.StackName

	params.collector.AddPreProcessor(&SimpleContainerArgs{}, func(c any) error {
		_, err = NewMongodbInitDbUserJob(ctx, userName, InitDbUserJobArgs{
			Namespace: namespace,
			User: DatabaseUser{
				Database: dbName,
				Username: userName,
				Password: password.Result,
			},
			RootUser:     params.connection.Username,
			RootPassword: params.connection.Password,
			Host:         params.connection.Host,
			Port:         params.connection.Port,
			InstanceName: params.instanceName,
			KubeProvider: params.provisionParams.Provider,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
		}
		return nil
	})

	return password, nil
}
