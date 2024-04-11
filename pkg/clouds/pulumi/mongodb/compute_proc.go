package mongodb

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func MongodbClusterComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	clusterName := input.ToResName(toClusterName(params.ParentStack.StackName, input))
	projectName := input.ToResName(toProjectName(params.ParentStack.StackName, input))

	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for mongodb atlas DB %q", params.ParentStack.RefString, input.Descriptor.Name)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--mongodb-atlas-ref", stack.Name, params.ParentStack.StackName), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.RefString).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	projectId, err := pApi.GetParentOutput(parentRef, toProjectIdExport(projectName), params.ParentStack.RefString, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get project id from parent stack for %q", stack.Name)
	}
	mongoUri, err := pApi.GetParentOutput(parentRef, toMongoUriWithOptionsExport(clusterName), params.ParentStack.RefString, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mongo uri from parent stack for %q", stack.Name)
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

			collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_USER")), userName,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_PASSWORD")), dbUser.Password,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)
			if strings.HasPrefix(mongoUri, "mongodb+srv://") {
				mongoUri = strings.ReplaceAll(mongoUri, "mongodb+srv://", fmt.Sprintf("mongodb+srv://%s:%s@", userName, dbUser.Password))
			} else {
				mongoUri = strings.ReplaceAll(mongoUri, "mongodb://", fmt.Sprintf("mongodb://%s:%s@", userName, dbUser.Password))
			}
			collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUri,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			return nil, nil
		}))
	}

	return &api.ResourceOutput{
		Ref: dbUser,
	}, nil
}
