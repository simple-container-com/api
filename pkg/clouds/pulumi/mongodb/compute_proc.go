package mongodb

import (
	"fmt"

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

	// Create a StackReference to the parent stack
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s-%s-mongodb-atlas-ref", stack.Name, params.ParentStack.StackName), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.RefString).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	projectId, err := getParentOutput(parentRef, toProjectIdExport(projectName), params.ParentStack.RefString)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get project id from parent stack for %q", stack.Name)
	}
	mongoUri, err := getParentOutput(parentRef, toMongoUriWithOptionsExport(clusterName), params.ParentStack.RefString)
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
	dbUser.Password.ApplyT(func(password *string) (any, error) {
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_USER")), userName)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_PASSWORD")), *password)
		collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUri)
		return nil, nil
	})

	return &api.ResourceOutput{
		Ref: dbUser,
	}, nil
}

func getParentOutput(ref *sdk.StackReference, outName string, parentRefString string) (string, error) {
	parentOutput, err := ref.GetOutputDetails(outName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get output %q from %q", outName, parentRefString)
	}
	if parentOutput.Value == nil {
		return "", errors.Wrapf(err, "no secret value for output %q from %q", outName, parentRefString)
	}
	if s, ok := parentOutput.Value.(string); ok {
		return s, nil
	} else {
		return "", errors.Wrapf(err, "parent output %q is not of type string (%T)", s, parentOutput.Value)
	}
}
