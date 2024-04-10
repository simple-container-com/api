package mongodb

import (
	"fmt"
	"net/url"

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
	clusterName := toClusterName(params.ParentStack.StackName, input)
	projectName := toProjectName(params.ParentStack.StackName, input)

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
		projectId: projectId,
		dbUri:     mongoUri,
		userName:  userName,
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
	ctx.Export(fmt.Sprintf("%s-username", userName), sdk.String(userName))
	ctx.Export(fmt.Sprintf("%s-password", userName), sdk.ToSecret(dbUser.Password))

	collector.AddDependency(dbUser)
	collector.AddOutput(dbUser.Password.ApplyT(func(password *string) (any, error) {
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_USER")), userName,
			input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_PASSWORD")), *password,
			input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)
		if mongoUrlParsed, err := url.Parse(mongoUri); err != nil {
			return nil, err
		} else {
			mongoUrlParsed.User = url.UserPassword(userName, *password)
			collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUrlParsed.String(),
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)
		}

		return nil, nil
	}))

	return &api.ResourceOutput{
		Ref: dbUser,
	}, nil
}
