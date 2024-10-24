package gcp

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/sql"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/util"
)

func PostgresComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}

	postgresName := toPostgresName(input, input.Descriptor.Name)
	fullParentReference := params.ParentStack.FullReference
	params.Log.Info(ctx.Context(), "Getting postgres root password for %q from parent stack %q", stack.Name, fullParentReference)
	rootPasswordExport := toPostgresRootPasswordExport(postgresName)
	rootPassword, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-cproc-rootpass", postgresName), fullParentReference, rootPasswordExport, true)
	if err != nil || rootPassword == "" {
		return nil, errors.Wrapf(err, "failed to get root password from parent stack for %q", postgresName)
	}

	pgCfg, ok := input.Descriptor.Config.Config.(*gcloud.PostgresGcpCloudsqlConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgresql config for %q", input.Descriptor.Type)
	}

	// TODO: move to provider init
	cloudresourcemanagerServiceName := fmt.Sprintf("projects/%s/services/cloudresourcemanager.googleapis.com", pgCfg.ProjectId)
	if err := enableServicesAPI(ctx.Context(), input.Descriptor.Config.Config, cloudresourcemanagerServiceName); err != nil {
		return nil, errors.Wrapf(err, "failed to enable %s", cloudresourcemanagerServiceName)
	}

	stackName := input.StackParams.StackName
	if pgCfg.UsersProvisionRuntime == nil {
		return nil, errors.Errorf("`usersProvisionRuntime` is not configured for %q in %q, so %q cannot consume it",
			input.Descriptor.Name, input.StackParams.Environment, stackName)
	}

	var kubeProvider *sdkK8s.Provider
	if pgCfg.UsersProvisionRuntime.Type == "gke" {
		clusterName := input.ToResName(pgCfg.UsersProvisionRuntime.ResourceName)
		params.Log.Info(ctx.Context(), "Getting kubeconfig for %q from parent stack %q", clusterName, fullParentReference)
		kubeConfig, err := pApi.GetStringValueFromStack(ctx, fmt.Sprintf("%s-cproc-kubeconfig", postgresName), fullParentReference, toKubeconfigExport(clusterName), true)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get kubeconfig from parent stack's resources")
		}
		if kubeConfig == "" {
			return nil, errors.Errorf("failed to get kubeconfig from parent stack's resources: empty")
		}
		kubeProviderName := fmt.Sprintf("%s-%s-computeproc-kubeconfig", input.ToResName(input.Descriptor.Name), clusterName)
		kubeProvider, err = sdkK8s.NewProvider(ctx, kubeProviderName, &sdkK8s.ProviderArgs{
			Kubeconfig: sdk.String(kubeConfig),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q",
				stackName, input.Descriptor.Name, input.StackParams.Environment)
		}
	} else {
		return nil, errors.Errorf("unsupported users provision runtime %q for %q/%q in %q",
			pgCfg.UsersProvisionRuntime.Type, stackName, input.Descriptor.Name, input.StackParams.Environment)
	}

	gcpProvider, ok := params.Provider.(*gcp.Provider)
	if !ok {
		return nil, errors.Errorf("failed to convert provider to *gcp.Provider when processing compute context for %q", postgresName)
	}
	appendContextParams := appendParams{
		config:          pgCfg,
		stack:           stack,
		collector:       collector,
		input:           input,
		rootPassword:    rootPassword,
		postgresName:    postgresName,
		provisionParams: params,
		kubeProvider:    kubeProvider,
		gcpProvider:     gcpProvider,
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

type appendParams struct {
	stack           api.Stack
	collector       pApi.ComputeContextCollector
	input           api.ResourceInput
	provisionParams pApi.ProvisionParams
	dependency      api.StackConfigDependencyResource
	rootPassword    string
	postgresName    string
	gcpProvider     *gcp.Provider
	kubeProvider    *sdkK8s.Provider
	config          *gcloud.PostgresGcpCloudsqlConfig
}

func appendUsesPostgresResourceContext(ctx *sdk.Context, params appendParams) error {
	// set both dbname and username to stack name
	dbName := params.stack.Name
	userName := params.stack.Name

	database, err := sql.NewDatabase(ctx, dbName, &sql.DatabaseArgs{
		Project:  sdk.String(params.config.Project),
		Instance: sdk.String(params.postgresName),
		Name:     sdk.String(dbName),
	}, sdk.Provider(params.gcpProvider))
	if err != nil {
		return errors.Wrapf(err, "failed to create database %q for stack %q", dbName, params.stack.Name)
	}

	password, err := createUserForDatabase(ctx, userName, dbName, params)
	if err != nil {
		return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
	}

	addCloudsqlProxySidecarPreProcessor(ctx, params)

	params.collector.AddOutput(sdk.All(password.Result, database.Name).ApplyT(func(args []any) (any, error) {
		userPassword := args[0].(string)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_USERNAME"), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_DATABASE"), dbName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_HOST"), "localhost",
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_PORT"), "5432",
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("POSTGRES_PASSWORD"), userPassword,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGHOST"), "localhost",
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGPORT"), "5432",
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
			"host":     "localhost",
			"port":     "5432",
		})

		return nil, nil
	}))

	return nil
}

func addCloudsqlProxySidecarPreProcessor(ctx *sdk.Context, params appendParams) {
	params.collector.AddPreProcessor(&kubernetes.SimpleContainerArgs{}, func(arg any) error {
		cloudsqlProxy, err := createCloudsqlProxy(ctx, params)
		if err != nil {
			return errors.Wrapf(err, "failed to create cloudsql proxy for %q in stack %q", params.postgresName, params.stack.Name)
		}

		kubeArgs, ok := arg.(*kubernetes.SimpleContainerArgs)
		if !ok {
			return errors.Errorf("arg is not *kubernetes.Args")
		}
		kubeArgs.SidecarOutputs = append(kubeArgs.SidecarOutputs, cloudsqlProxy.ProxyContainer.ApplyT(func(arg any) corev1.ContainerArgs {
			return arg.(corev1.ContainerArgs)
		}).(corev1.ContainerOutput))
		kubeArgs.VolumeOutputs = append(kubeArgs.VolumeOutputs, cloudsqlProxy.SqlProxySecret.Metadata.Name().ApplyT(func(arg any) corev1.VolumeArgs {
			return corev1.VolumeArgs{
				Name: sdk.String(lo.FromPtr(arg.(*string))),
				Secret: &corev1.SecretVolumeSourceArgs{
					SecretName: sdk.StringPtrFromPtr(arg.(*string)),
				},
			}
		}).(corev1.VolumeOutput))
		return nil
	})
}

func createCloudsqlProxy(ctx *sdk.Context, params appendParams) (*CloudSQLProxy, error) {
	cloudsqlProxyName := fmt.Sprintf("%s-%s-sidecarcsql", params.stack.Name, params.postgresName)
	cloudsqlProxy, err := NewCloudsqlProxy(ctx, CloudSQLProxyArgs{
		Name: cloudsqlProxyName,
		DBInstance: PostgresDBInstanceArgs{
			Project:      params.config.ProjectId,
			InstanceName: params.postgresName,
			Region:       lo.FromPtr(params.config.Region),
		},
		GcpProvider:  params.gcpProvider,
		KubeProvider: params.kubeProvider,
		Metadata:     cloudsqlProxyMeta(params.input.StackParams.StackName, cloudsqlProxyName, params),
	})
	if err != nil {
		return nil, err
	}
	params.collector.AddDependency(cloudsqlProxy.Account.ServiceAccount)
	params.collector.AddDependency(cloudsqlProxy.Account.ServiceAccountKey)
	params.collector.AddDependency(cloudsqlProxy.SqlProxySecret)
	params.collector.AddOutput(cloudsqlProxy.ProxyContainer)
	return cloudsqlProxy, nil
}

func appendDependsOnPostgresResourceContext(ctx *sdk.Context, params appendParams) error {
	ownerStackName := pApi.CollapseStackReference(params.dependency.Owner)
	userName := fmt.Sprintf("%s--%s", params.stack.Name, params.dependency.Name)
	dbName := pApi.StackNameInEnv(ownerStackName, params.input.StackParams.Environment)

	password, err := createUserForDatabase(ctx, userName, dbName, params)
	if err != nil {
		return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
	}

	addCloudsqlProxySidecarPreProcessor(ctx, params)

	params.collector.AddOutput(sdk.All(password.Result).ApplyT(func(args []any) (any, error) {
		userPassword := args[0].(string)

		params.collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_PASSWORD", ownerStackName)), userPassword,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_USERNAME", ownerStackName)), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_DATABASE", dbName)), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_HOST", "localhost")), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)
		params.collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("POSTGRES_DEP_%s_PORT", "5432")), userName,
			params.input.Descriptor.Type, params.input.Descriptor.Name, params.provisionParams.ParentStack.StackName)

		params.collector.AddDependencyTplExtension(params.dependency.Name, params.input.Descriptor.Name, map[string]string{
			"password": userPassword,
			"user":     userName,
			"database": dbName,
			"host":     "localhost",
			"port":     "5432",
		})

		return nil, nil
	}))

	return nil
}

func createUserForDatabase(ctx *sdk.Context, userName, dbName string, params appendParams) (*random.RandomPassword, error) {
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

	_, err = sql.NewUser(ctx, fmt.Sprintf("%s-%s-user", userName, params.postgresName), &sql.UserArgs{
		Password: password.Result,
		Project:  sdk.String(params.config.Project),
		Instance: sdk.String(params.postgresName),
		Name:     sdk.String(userName),
	}, sdk.Provider(params.gcpProvider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create database user %q for database %q in stack %q", userName, dbName, params.stack.Name)
	}

	params.collector.AddPostProcessor(&kubernetes.SimpleContainer{}, func(c any) error {
		sc := c.(*kubernetes.SimpleContainer)
		dbInstanceArgs := PostgresDBInstanceArgs{
			Project:      params.config.ProjectId,
			InstanceName: params.postgresName,
			Region:       lo.FromPtr(params.config.Region),
		}
		cloudsqlProxyName := util.TrimStringMiddle(fmt.Sprintf("%s-%s-initcsql", userName, params.postgresName), 60, "-")
		namespace := params.input.StackParams.StackName
		cloudsqlProxy, err := NewCloudsqlProxy(ctx, CloudSQLProxyArgs{
			Name:         cloudsqlProxyName,
			DBInstance:   dbInstanceArgs,
			GcpProvider:  params.gcpProvider,
			KubeProvider: params.kubeProvider,
			TimeoutSec:   MaxInitSQLTimeSec,
			Metadata:     cloudsqlProxyMeta(namespace, cloudsqlProxyName, params),
		}, sdk.DependsOn([]sdk.Resource{sc}))
		if err != nil {
			return errors.Wrapf(err, "failed to init cloudsql proxy")
		}

		_, err = NewInitDbUserJob(ctx, userName, InitDbUserJobArgs{
			Namespace: namespace,
			User: CloudsqlDbUser{
				Database: dbName,
				Username: userName,
			},
			RootPassword:   params.rootPassword,
			DBInstance:     dbInstanceArgs,
			CloudSQLProxy:  cloudsqlProxy,
			KubeProvider:   params.kubeProvider,
			DBInstanceType: PostgreSQL,
			Opts:           []sdk.ResourceOption{sdk.DependsOn([]sdk.Resource{cloudsqlProxy.SqlProxySecret, sc})},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to init user %q for database %q", userName, dbName)
		}
		return nil
	})

	return password, nil
}

func cloudsqlProxyMeta(namespace string, cloudsqlProxyName string, params appendParams) *v1.ObjectMetaArgs {
	return &v1.ObjectMetaArgs{
		Namespace: sdk.String(namespace),
		Name:      sdk.String(cloudsqlProxyName),
		Labels: sdk.StringMap{
			kubernetes.LabelAppType: sdk.String(kubernetes.AppTypeSimpleContainer),
			kubernetes.LabelScEnv:   sdk.String(params.input.StackParams.Environment),
			kubernetes.LabelAppName: sdk.String(params.input.StackParams.StackName),
		},
	}
}
