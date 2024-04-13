package mongodb

import (
	"encoding/json"
	"fmt"

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

	// set both dbname and user name to stack name
	dbName := stack.Name
	userName := stack.Name

	dbUser, err := createDatabaseUser(ctx, dbUserInput{
		clusterName: clusterName,
		projectId:   projectId,
		dbUri:       mongoUri,
		userName:    userName,
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
				dbName: "admin",
				role:   "readAnyDatabase",
			},
		},
	}, params)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create service user for database %q", dbName)
	}
	if dbUser != nil {
		ctx.Export(fmt.Sprintf("%s-service-user", clusterName), dbUser.(sdk.Output))

		collector.AddOutput(dbUser.(sdk.Output).ApplyT(func(dbUserOut any) (any, error) {
			dbUserOutJson := dbUserOut.(string)
			dbUser := DbUserOutput{}
			_ = json.Unmarshal([]byte(dbUserOutJson), &dbUser)

			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_USER")), userName,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_PASSWORD")), dbUser.Password,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			mongoUri = appendUserPasswordAndDBToMongoUri(mongoUri, userName, dbUser.Password, dbName)

			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUri,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
				"uri":      mongoUri,
				"password": dbUser.Password,
				"user":     userName,
			})

			return nil, nil
		}))
	}

	return &api.ResourceOutput{
		Ref: dbUser,
	}, nil
}
