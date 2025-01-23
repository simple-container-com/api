package kubernetes

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

func HelmPostgresOperatorComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	postgresName := toPostgresInstanceName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference
	params.Log.Info(ctx.Context(), "Getting postgres root password for %q from parent stack %q", stack.Name, fullParentReference)
	rootPasswordExport := toPostgresRootPasswordExport(postgresName)
	rootPassword, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-cproc-rootpass", postgresName), fullParentReference, rootPasswordExport, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get root password from parent stack for %q", postgresName)
	} else if rootPassword == "" {
		return nil, errors.Errorf("failed to get root password (empty) from parent stack for %q", postgresName)
	}
	rootUserExport := toPostgresRootUsernameExport(postgresName)
	rootUser, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-cproc-rootuser", postgresName), fullParentReference, rootUserExport, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get root user from parent stack for %q", postgresName)
	} else if rootUser == "" {
		return nil, errors.Errorf("failed to get root user (empty) from parent stack for %q", postgresName)
	}
	pgURLExport := toPostgresRootURLExport(postgresName)
	pgURL, err := pApi.GetValueFromStack[string](ctx, fmt.Sprintf("%s-cproc-pg-url", postgresName), fullParentReference, pgURLExport, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres URL from parent stack for %q", postgresName)
	} else if pgURL == "" {
		return nil, errors.Errorf("failed to get postgres URL (empty) from parent stack for %q", postgresName)
	}

	appendContextParams := postgresAppendParams{
		stack:           stack,
		collector:       collector,
		input:           input,
		rootUser:        rootUser,
		rootPassword:    rootPassword,
		postgresName:    postgresName,
		pgURL:           pgURL,
		provisionParams: params,
	}
	if params.UseResources[input.Descriptor.Name] {
		if err := appendUsesPostgresResourceContext(ctx, appendContextParams); err != nil {
			return nil, errors.Wrapf(err, "failed to append consumes resource context")
		}
	}

	for _, dep := range lo.Filter(params.DependOnResources, func(d api.StackConfigDependencyResource, _ int) bool {
		return d.Resource == input.Descriptor.Name
	}) {
		appendContextParams.dependency = dep
		if err := appendDependsOnPostgresResourceContext(ctx, appendContextParams); err != nil {
			return nil, err
		}
	}

	return &api.ResourceOutput{
		Ref: nil,
	}, nil
}

type postgresAppendParams struct {
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	provisionParams pApi.ProvisionParams
	dependency      api.StackConfigDependencyResource
	rootUser        string
	rootPassword    string
	postgresName    string
	pgURL           string
}

func appendUsesPostgresResourceContext(ctx *sdk.Context, params postgresAppendParams) error {
	// set both dbname and username to stack name
	dbName := params.stack.Name
	userName := params.stack.Name

	password, err := createPostgresUserForDatabase(ctx, userName, dbName, params)
	if err != nil {
		return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
	}
	parsedPgURL, err := url.Parse(params.pgURL)
	if err != nil {
		return errors.Wrapf(err, "failed to parse postgres url for database %q", dbName)
	}

	params.collector.AddOutput(sdk.All(password.Result, dbName).ApplyT(func(args []any) (any, error) {
		userPassword := args[0].(string)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_USERNAME"), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_DATABASE"), dbName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_HOST"), parsedPgURL.Hostname(),
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_PORT"), parsedPgURL.Port(),
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_PASSWORD"), userPassword,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("PGSSLMODE"), "require",
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGHOST"), parsedPgURL.Hostname(),
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGPORT"), parsedPgURL.Port(),
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGDATABASE"), dbName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGUSER"), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("PGPASSWORD"), userPassword,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddResourceTplExtension(params.input.Descriptor.Name, map[string]string{
			"password": userPassword,
			"user":     userName,
			"database": dbName,
			"host":     parsedPgURL.Hostname(),
			"port":     parsedPgURL.Port(),
		})

		return nil, nil
	}))

	return nil
}

func appendDependsOnPostgresResourceContext(ctx *sdk.Context, params postgresAppendParams) error {
	ownerStackName := pApi.CollapseStackReference(params.dependency.Owner)
	userName := fmt.Sprintf("%s--%s", params.stack.Name, params.dependency.Name)
	dbName := pApi.StackNameInEnv(ownerStackName, params.input.StackParams.Environment)

	password, err := createPostgresUserForDatabase(ctx, userName, dbName, params)
	if err != nil {
		return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
	}
	parsedPgURL, err := url.Parse(params.pgURL)
	if err != nil {
		return errors.Wrapf(err, "failed to parse postgres url for database %q", dbName)
	}

	params.collector.AddOutput(sdk.All(password.Result).ApplyT(func(args []any) (any, error) {
		userPassword := args[0].(string)

		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_PASSWORD", ownerStackName)), userPassword,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_USERNAME", ownerStackName)), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_DATABASE", dbName)), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_HOST", parsedPgURL.Hostname())), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_PORT", parsedPgURL.Port())), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddDependencyTplExtension(params.dependency.Name, params.input.Descriptor.Name, map[string]string{
			"password": userPassword,
			"user":     userName,
			"database": dbName,
			"host":     parsedPgURL.Hostname(),
			"port":     parsedPgURL.Port(),
		})

		return nil, nil
	}))

	return nil
}

func createPostgresUserForDatabase(ctx *sdk.Context, userName, dbName string, params postgresAppendParams) (*random.RandomPassword, error) {
	ctx.Export(fmt.Sprintf("%s-%s-username", userName, params.postgresName), sdk.String(userName))
	passwordName := fmt.Sprintf("%s-%s-password", userName, params.postgresName)
	password, err := random.NewRandomPassword(ctx, passwordName, &random.RandomPasswordArgs{
		Length:  sdk.Int(20),
		Special: sdk.Bool(false),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate random password for postgres for user %q", userName)
	}
	ctx.Export(passwordName, password.Result)

	parsedPgURL, err := url.Parse(params.pgURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse postgres url for database %q", dbName)
	}

	namespace := params.input.StackParams.StackName

	params.collector.AddPreProcessor(&SimpleContainerArgs{}, func(c any) error {
		_, err = NewPostgresInitDbUserJob(ctx, userName, InitDbUserJobArgs{
			Namespace: namespace,
			User: DatabaseUser{
				Database: dbName,
				Username: userName,
				Password: password.Result,
			},
			RootUser:     params.rootUser,
			RootPassword: params.rootPassword,
			Host:         parsedPgURL.Hostname(),
			Port:         parsedPgURL.Port(),
			KubeProvider: params.provisionParams.Provider,
			InstanceName: params.postgresName,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
		}
		return nil
	})

	return password, nil
}
