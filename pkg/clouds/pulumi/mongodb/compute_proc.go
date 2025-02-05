package mongodb

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func ClusterComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	projectName := toProjectName(params.ParentStack.StackName, input)
	clusterName := toClusterName(params.ParentStack.StackName, input)

	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for mongodb atlas cluster %q (%q)", params.ParentStack.FullReference, clusterName, suffix)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s%s--mongodb-atlas-ref", stack.Name, input.Descriptor.Name, params.ParentStack.FullReference, suffix),
		&sdk.StackReferenceArgs{
			Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
		})
	if err != nil {
		return nil, err
	}

	projectIdExport := toProjectIdExport(projectName)
	projectId, err := pApi.GetParentOutput(parentRef, projectIdExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get project id from parent stack for %q (%q)", stack.Name, projectIdExport)
	} else if projectId == "" {
		return nil, errors.Errorf("project id is empty for %q (%q)", stack.Name, projectIdExport)
	}
	mongoUriExport := toMongoUriWithOptionsExport(clusterName)
	mongoUri, err := pApi.GetParentOutput(parentRef, mongoUriExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mongo uri from parent stack for %q (%q)", stack.Name, mongoUriExport)
	} else if mongoUri == "" {
		return nil, errors.Errorf("mongo uri is empty for %q (%q)", stack.Name, mongoUriExport)
	}

	appendContextParams := appendParams{
		stack:           stack,
		collector:       collector,
		input:           input,
		clusterName:     clusterName,
		projectName:     projectName,
		projectId:       projectId,
		mongoUri:        mongoUri,
		provisionParams: params,
		suffix:          suffix,
	}
	if params.ParentStack.UsesResource {
		if err := appendUsesResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	} else if params.ParentStack.DependsOnResource != nil {
		appendContextParams.dependency = *params.ParentStack.DependsOnResource
		if err := appendDependsOnResourceContext(ctx, appendContextParams); err != nil {
			return nil, err
		}
	} else {
		params.Log.Warn(ctx.Context(), "mongodb %q only supports `uses` or `dependency`, but neither was explicitly declared as being used", clusterName)
		return nil, errors.Errorf("mongodb %q only supports `uses` or `dependency`, but it wasn't explicitly declared as being used", clusterName)
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

type appendParams struct {
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	clusterName     string
	projectName     string
	projectId       string
	mongoUri        string
	provisionParams pApi.ProvisionParams
	dependency      api.StackConfigDependencyResource
	suffix          string
}

func appendUsesResourceContext(ctx *sdk.Context, params appendParams) error {
	// set both dbname and user name to stack name
	dbName := params.stack.Name
	userName := params.stack.Name

	dbUser, err := createDatabaseUser(ctx, dbUserInput{
		clusterName: params.clusterName,
		projectId:   params.projectId,
		dbUri:       params.mongoUri,
		username:    params.stack.Name,
		roles: []dbRole{
			{
				dbName: dbName,
				role:   "dbAdmin",
			},
			{
				dbName: dbName,
				role:   "readWrite",
			},
			{
				dbName: "local",
				role:   "read",
			},
		},
		suffix: params.suffix,
	}, params.provisionParams)
	if err != nil {
		return errors.Wrapf(err, "failed to create service user for database %q", dbName)
	}
	if dbUser != nil {
		ctx.Export(fmt.Sprintf("%s%s-service-user", params.clusterName, params.suffix), dbUser)

		params.collector.AddOutput(ctx, dbUser.ApplyT(func(dbUserOut any) (any, error) {
			dbUserOutJson, ok := dbUserOut.(string)
			if !ok {
				return nil, errors.Errorf("db user is not a string for mongodb user %q", userName)
			}
			dbUser := DbUserOutput{}
			err = json.Unmarshal([]byte(dbUserOutJson), &dbUser)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal db user for mongodb user %q", userName)
			}

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_USER"), userName,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_DATABASE"), dbName,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_PASSWORD"), dbUser.Password,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			mongoUri := AppendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, dbName)

			// oplog uri is necessary for apps that would like to read mongo's oplog
			oplogMongoUri := AppendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, "local")

			params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("MONGO_URI"), mongoUri,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
				"uri":      mongoUri,
				"dbName":   dbName,
				"password": dbUser.Password,
				"user":     userName,
				"oplogUri": oplogMongoUri,
			})

			return nil, nil
		}))
	}

	return nil
}

func appendDependsOnResourceContext(ctx *sdk.Context, params appendParams) error {
	ownerStackName := pApi.CollapseStackReference(params.dependency.Owner)
	userName := fmt.Sprintf("%s--%s%s", params.stack.Name, params.dependency.Name, params.suffix)
	dbEnv := lo.If(params.input.StackParams.ParentEnv != "", params.input.StackParams.ParentEnv).Else(params.input.StackParams.Environment)
	dbName := pApi.StackNameInEnv(ownerStackName, dbEnv)

	dbUser, err := createDatabaseUser(ctx, dbUserInput{
		clusterName: params.clusterName,
		projectId:   params.projectId,
		dbUri:       params.mongoUri,
		username:    userName,
		roles: []dbRole{
			{
				dbName: dbName,
				role:   "dbAdmin",
			},
			{
				dbName: dbName,
				role:   "readWrite",
			},
			{
				dbName: "local",
				role:   "read",
			},
		},
		suffix: params.suffix,
	}, params.provisionParams)
	if err != nil {
		return errors.Wrapf(err, "failed to create service user for database %q", dbName)
	}
	if dbUser != nil {
		ctx.Export(fmt.Sprintf("%s--to--%s--%s%s", params.clusterName, ownerStackName, params.dependency.Resource, params.suffix), dbUser)

		params.collector.AddOutput(ctx, dbUser.ApplyT(func(dbUserOut any) (any, error) {
			params.provisionParams.Log.Info(ctx.Context(), "Creating mongo user %q", userName)
			dbUserOutJson, ok := dbUserOut.(string)
			if !ok {
				return nil, errors.Errorf("db user is not a string")
			}
			dbUser := DbUserOutput{}
			err = json.Unmarshal([]byte(dbUserOutJson), &dbUser)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to unmarshal db user for mongo %q", userName)
			}

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_USER", ownerStackName)), userName,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_PASSWORD", ownerStackName)), dbUser.Password,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			mongoUri := AppendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, dbName)

			// oplog uri is necessary for apps that would like to read mongo's oplog
			oplogMongoUri := AppendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, "local")

			params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_URI", ownerStackName)), mongoUri,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddDependencyTplExtension(params.dependency.Name, params.input.Descriptor.Name, map[string]string{
				"uri":      mongoUri,
				"dbName":   dbName,
				"password": dbUser.Password,
				"user":     userName,
				"oplogUri": oplogMongoUri,
			})

			return nil, nil
		}))
	}

	return nil
}
