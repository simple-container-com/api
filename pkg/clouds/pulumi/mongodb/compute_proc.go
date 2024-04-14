package mongodb

import (
	"encoding/json"
	"fmt"

	"github.com/samber/lo"

	"github.com/pkg/errors"
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

	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for mongodb atlas DB %q", params.ParentStack.FulReference, input.Descriptor.Name)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s--mongodb-atlas-ref", stack.Name, input.Descriptor.Name, params.ParentStack.StackName),
		&sdk.StackReferenceArgs{
			Name: sdk.String(params.ParentStack.FulReference).ToStringOutput(),
		})
	if err != nil {
		return nil, err
	}

	projectIdExport := toProjectIdExport(projectName)
	projectId, err := pApi.GetParentOutput(parentRef, projectIdExport, params.ParentStack.FulReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get project id from parent stack for %q (%q)", stack.Name, projectIdExport)
	} else if projectId == "" {
		return nil, errors.Errorf("project id is empty for %q (%q)", stack.Name, projectIdExport)
	}
	mongoUriExport := toMongoUriWithOptionsExport(clusterName)
	mongoUri, err := pApi.GetParentOutput(parentRef, mongoUriExport, params.ParentStack.FulReference, false)
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
	}
	if params.UseResources[input.Descriptor.Name] {
		if err := appendUsesResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	}

	for _, dep := range lo.Filter(params.DependOnResources, func(d api.StackConfigDependencyResource, _ int) bool {
		return d.Resource == input.Descriptor.Name
	}) {
		appendContextParams.dependency = dep
		if err := appendDependsOnResourceContext(ctx, appendContextParams); err != nil {
			return nil, err
		}
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
}

func appendUsesResourceContext(ctx *sdk.Context, params appendParams) error {
	// set both dbname and user name to stack name
	dbName := params.stack.Name
	userName := params.stack.Name

	dbUser, err := createDatabaseUser(ctx, dbUserInput{
		clusterName: params.clusterName,
		projectId:   params.projectId,
		dbUri:       params.mongoUri,
		userName:    params.stack.Name,
		roles: []dbRole{
			{
				dbName: dbName,
				role:   "dbAdmin",
			},
			{
				dbName: dbName,
				role:   "readWrite",
			},
		},
	}, params.provisionParams)
	if err != nil {
		return errors.Wrapf(err, "failed to create service user for database %q", dbName)
	}
	if dbUser != nil {
		ctx.Export(fmt.Sprintf("%s-service-user", params.clusterName), dbUser.(sdk.Output))

		params.collector.AddOutput(dbUser.(sdk.Output).ApplyT(func(dbUserOut any) (any, error) {
			dbUserOutJson := dbUserOut.(string)
			dbUser := DbUserOutput{}
			_ = json.Unmarshal([]byte(dbUserOutJson), &dbUser)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_USER")), userName,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_PASSWORD")), dbUser.Password,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			mongoUri := appendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, dbName)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUri,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
				"uri":      mongoUri,
				"password": dbUser.Password,
				"user":     userName,
			})

			return nil, nil
		}))
	}

	return nil
}

func appendDependsOnResourceContext(ctx *sdk.Context, params appendParams) error {
	ownerStackName := pApi.CollapseStackReference(params.dependency.Owner)
	userName := fmt.Sprintf("%s-to-%s", params.stack.Name, ownerStackName)
	dbName := pApi.StackNameInEnv(ownerStackName, params.input.StackParams.Environment)

	dbUser, err := createDatabaseUser(ctx, dbUserInput{
		clusterName: params.clusterName,
		projectId:   params.projectId,
		dbUri:       params.mongoUri,
		userName:    params.stack.Name,
		roles: []dbRole{
			{
				dbName: dbName,
				role:   "dbAdmin",
			},
			{
				dbName: dbName,
				role:   "readWrite",
			},
		},
	}, params.provisionParams)
	if err != nil {
		return errors.Wrapf(err, "failed to create service user for database %q", dbName)
	}
	if dbUser != nil {
		ctx.Export(fmt.Sprintf("%s-d-%s", params.clusterName, ownerStackName), dbUser.(sdk.Output))

		params.collector.AddOutput(dbUser.(sdk.Output).ApplyT(func(dbUserOut any) (any, error) {
			dbUserOutJson := dbUserOut.(string)
			dbUser := DbUserOutput{}
			_ = json.Unmarshal([]byte(dbUserOutJson), &dbUser)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_USER", ownerStackName)), userName,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_PASSWORD", ownerStackName)), dbUser.Password,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			mongoUri := appendUserPasswordAndDBToMongoUri(params.mongoUri, userName, dbUser.Password, dbName)

			params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_DEP_%s_URI", ownerStackName)), mongoUri,
				params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

			params.collector.AddDependencyTplExtension(params.dependency.Name, params.input.Descriptor.Name, map[string]string{
				"uri":      mongoUri,
				"password": dbUser.Password,
				"user":     userName,
			})

			return nil, nil
		}))
	}

	return nil
}
