package gcp

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/projects"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/util"
)

type PostgresDBInstanceArgs struct {
	Project      string
	InstanceName string
	Region       string
}

type CloudSQLAccount struct {
	ServiceAccount     *serviceaccount.Account
	ServiceAccountKey  *serviceaccount.Key
	CredentialsSecrets sdk.StringMap
}

func NewCloudSQLAccount(ctx *sdk.Context, name string, dbInstance PostgresDBInstanceArgs, provider *gcp.Provider) (*CloudSQLAccount, error) {
	accountName := strings.ReplaceAll(util.TrimStringMiddle(name, 28, "-"), "--", "-")

	serviceAccount, err := serviceaccount.NewAccount(ctx, accountName, &serviceaccount.AccountArgs{
		AccountId:   sdk.String(accountName),
		Project:     sdk.String(dbInstance.Project),
		Description: sdk.String(fmt.Sprintf("Service account to access database %s", dbInstance.InstanceName)),
		DisplayName: sdk.String(fmt.Sprintf("%s-service-account", name)),
	}, sdk.Provider(provider))
	if err != nil {
		return nil, err
	}

	serviceAccountKey, err := serviceaccount.NewKey(ctx, fmt.Sprintf("%s-key", accountName), &serviceaccount.KeyArgs{
		ServiceAccountId: serviceAccount.AccountId,
	}, sdk.Parent(serviceAccount), sdk.Provider(provider))
	if err != nil {
		return nil, err
	}

	_, err = projects.NewIAMMember(ctx, fmt.Sprintf("%s-iam", accountName), &projects.IAMMemberArgs{
		Project: sdk.String(dbInstance.Project),
		Member:  serviceAccount.Member,
		Role:    sdk.String("roles/cloudsql.client"),
	}, sdk.Parent(serviceAccount), sdk.Provider(provider))
	if err != nil {
		return nil, err
	}

	credentialsSecrets := sdk.StringMap{
		"credentials.json": serviceAccountKey.PrivateKey,
	}

	return &CloudSQLAccount{
		ServiceAccount:     serviceAccount,
		ServiceAccountKey:  serviceAccountKey,
		CredentialsSecrets: credentialsSecrets,
	}, nil
}

type CloudSQLProxyArgs struct {
	Name         string
	DBInstance   PostgresDBInstanceArgs
	GcpProvider  *gcp.Provider
	KubeProvider *sdkK8s.Provider
	Metadata     *metav1.ObjectMetaArgs
	TimeoutSec   int
}

type CloudSQLProxy struct {
	ProxyContainer sdk.Output
	Account        *CloudSQLAccount
	Name           string
	SqlProxySecret *v1.Secret
}

func NewCloudsqlProxy(ctx *sdk.Context, args CloudSQLProxyArgs) (*CloudSQLProxy, error) {
	account, err := NewCloudSQLAccount(ctx, args.Name, args.DBInstance, args.GcpProvider)
	if err != nil {
		return nil, err
	}

	sqlProxySecret, err := v1.NewSecret(ctx, args.Name+"-creds", &v1.SecretArgs{
		Metadata: args.Metadata,
		Data:     account.CredentialsSecrets,
	}, sdk.Provider(args.KubeProvider))
	if err != nil {
		return nil, err
	}

	proxyContainer := cloudsqlProxyContainer(sqlProxySecret, args.DBInstance, args.TimeoutSec)

	return &CloudSQLProxy{
		ProxyContainer: proxyContainer,
		Account:        account,
		Name:           args.Name,
		SqlProxySecret: sqlProxySecret,
	}, nil
}

func cloudsqlProxyContainer(credsSecret *v1.Secret, dbInstance PostgresDBInstanceArgs, timeout int) sdk.Output {
	return sdk.All(credsSecret.Metadata.Name(), dbInstance.Project, dbInstance.Region, dbInstance.InstanceName).ApplyT(func(all []interface{}) v1.ContainerArgs {
		secretName := all[0].(*string)
		project := all[1].(string)
		region := all[2].(string)
		instanceName := all[3].(string)

		command := "/cloud-sql-proxy"
		args := []string{
			"--address",
			"0.0.0.0",
			"--structured-logs",
			"--credentials-file=/var/run/secrets/cloudsql/credentials.json",
			fmt.Sprintf("%s:%s:%s", project, region, instanceName),
		}

		if timeout > 0 {
			args = []string{
				"-c",
				fmt.Sprintf(`
                    echo "Starting proxy with timeout %ds..."
                    %s %s &
                    PROXY_PID=$!
                    
                    echo "Waiting %ds until killing proxy..."
                    sleep %d;
                    
                    echo "Killing proxy after %ds"
                    kill -9 $PROXY_PID;
                    exit 0;
                `, timeout, command, strings.Join(args, " "), timeout, timeout, timeout),
			}
			command = "sh"
		}

		return v1.ContainerArgs{
			Name:  sdk.String("cloudsql-proxy"),
			Image: sdk.String("gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.8.1-alpine"),
			Command: sdk.StringArray{
				sdk.String(command),
			},
			Args: sdk.ToStringArray(args),
			SecurityContext: &v1.SecurityContextArgs{
				RunAsNonRoot: sdk.Bool(true),
			},
			Resources: &v1.ResourceRequirementsArgs{
				Limits: sdk.StringMap{
					"memory": sdk.String("300Mi"),
					"cpu":    sdk.String("300m"),
				},
				Requests: sdk.StringMap{
					"memory": sdk.String("200Mi"),
					"cpu":    sdk.String("50m"),
				},
			},
			VolumeMounts: v1.VolumeMountArray{
				&v1.VolumeMountArgs{
					Name:      sdk.String(lo.FromPtr(secretName)),
					MountPath: sdk.String("/var/run/secrets/cloudsql"),
					ReadOnly:  sdk.Bool(true),
				},
			},
		}
	}).(v1.ContainerOutput)
}
