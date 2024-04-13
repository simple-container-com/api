package mongodb

import (
	"encoding/json"
	"fmt"
	"github.com/simple-container-com/welder/pkg/template"
	"strings"

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
			mongoUri = appendUserPasswordToMongoUri(mongoUri, userName, dbUser.Password)
			collector.AddEnvVariable(util.ToEnvVariableName(fmt.Sprintf("MONGO_URI")), mongoUri,
				input.Descriptor.Type, input.Descriptor.Name, params.ParentStack.StackName)

			collector.AddTplExtensions(map[string]template.Extension{
				"resource": func(noSubs string, path string, defaultValue *string) (string, error) {
					pathParts := strings.SplitN(path, ".", 2)
					refResName := pathParts[0]
					if refResName != input.Descriptor.Name {
						return noSubs, nil
					}
					refValue := pathParts[1]
					if value, ok := map[string]string{
						"uri":      mongoUri,
						"password": dbUser.Password,
						"user":     userName,
					}[refValue]; ok {
						return value, nil
					}
					return noSubs, nil
				},
			})

			return nil, nil
		}))
	}

	return &api.ResourceOutput{
		Ref: dbUser,
	}, nil
}
