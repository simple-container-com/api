package gcp

import (
	"fmt"

	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp/serviceaccount"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type PostgresDBInstanceArgs struct {
	Project      string
	InstanceName string
	Region       string
}

type CloudSQLAccount struct {
	ServiceAccount     *serviceaccount.Account
	ServiceAccountKey  *serviceaccount.Key
	CredentialsSecrets pulumi.StringMap
}

func NewCloudSQLAccount(ctx *pulumi.Context, name string, dbInstance PostgresDBInstanceArgs, provider *gcp.Provider) (*CloudSQLAccount, error) {
	accountName := name

	serviceAccount, err := serviceaccount.NewAccount(ctx, accountName, &serviceaccount.AccountArgs{
		AccountId: pulumi.String(accountName),
		Project:   pulumi.String(dbInstance.Project),
	}, pulumi.Provider(provider))
	if err != nil {
		return nil, err
	}

	serviceAccountKey, err := serviceaccount.NewKey(ctx, fmt.Sprintf("%s-key", accountName), &serviceaccount.KeyArgs{
		ServiceAccountId: serviceAccount.ID(),
	}, pulumi.Parent(serviceAccount), pulumi.Provider(provider))
	if err != nil {
		return nil, err
	}

	_, err = serviceaccount.NewIAMMember(ctx, fmt.Sprintf("%s-iam", accountName), &serviceaccount.IAMMemberArgs{
		Member: serviceAccount.Email.ApplyT(func(email string) string { return fmt.Sprintf("serviceAccount:%s", email) }).(pulumi.StringOutput),
		Role:   pulumi.String("roles/cloudsql.client"),
	}, pulumi.Parent(serviceAccount), pulumi.Provider(provider))
	if err != nil {
		return nil, err
	}

	credentialsSecrets := pulumi.StringMap{
		"credentials.json": serviceAccountKey.PrivateKey,
	}

	return &CloudSQLAccount{
		ServiceAccount:     serviceAccount,
		ServiceAccountKey:  serviceAccountKey,
		CredentialsSecrets: credentialsSecrets,
	}, nil
}

type CloudSQLProxyArgs struct {
	Name       string
	DBInstance PostgresDBInstanceArgs
	Provider   *gcp.Provider
	Metadata   *metav1.ObjectMetaArgs
	TimeoutSec int
}

type CloudSQLProxy struct {
	ProxyContainer pulumi.Output
	Account        *CloudSQLAccount
	Name           string
	SqlProxySecret *v1.Secret
}

func NewCloudsqlProxy(ctx *pulumi.Context, args CloudSQLProxyArgs) (*CloudSQLProxy, error) {
	account, err := NewCloudSQLAccount(ctx, args.Name, args.DBInstance, args.Provider)
	if err != nil {
		return nil, err
	}

	sqlProxySecret, err := v1.NewSecret(ctx, args.Name+"-creds", &v1.SecretArgs{
		Metadata: args.Metadata,
		Data:     account.CredentialsSecrets,
	}, pulumi.Provider(args.Provider))
	if err != nil {
		return nil, err
	}

	proxyContainer := cloudsqlProxyContainer(args.DBInstance, args.TimeoutSec)

	return &CloudSQLProxy{
		ProxyContainer: proxyContainer,
		Account:        account,
		Name:           args.Name,
		SqlProxySecret: sqlProxySecret,
	}, nil
}

func cloudsqlProxyContainer(dbInstance PostgresDBInstanceArgs, timeout int) v1.ContainerOutput {
	return pulumi.All(dbInstance.Project, dbInstance.Region, dbInstance.InstanceName).ApplyT(func(all []interface{}) v1.ContainerArgs {
		project := all[0].(string)
		region := all[1].(string)
		instanceName := all[2].(string)

		command := "/cloud-sql-proxy"
		args := []string{
			"--address",
			"0.0.0.0",
			"--structured-logs",
			"--credentials-file=/var/run/secrets/cloudsql/credentials.json",
			fmt.Sprintf("%s:%s:%s", project, region, instanceName),
		}

		if timeout > 0 {
			command = "sh"
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
                `, timeout, command, args, timeout, timeout, timeout),
			}
		}

		return v1.ContainerArgs{
			Name:  pulumi.String("cloudsql-proxy"),
			Image: pulumi.String("gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.8.1-alpine"),
			Command: pulumi.StringArray{
				pulumi.String(command),
			},
			Args: pulumi.ToStringArray(args),
			SecurityContext: &v1.SecurityContextArgs{
				RunAsNonRoot: pulumi.Bool(true),
			},
			Resources: &v1.ResourceRequirementsArgs{
				Limits: pulumi.StringMap{
					"memory": pulumi.String("300Mi"),
					"cpu":    pulumi.String("300m"),
				},
				Requests: pulumi.StringMap{
					"memory": pulumi.String("200Mi"),
					"cpu":    pulumi.String("50m"),
				},
			},
			VolumeMounts: v1.VolumeMountArray{
				&v1.VolumeMountArgs{
					Name:      pulumi.String("cloudsql-creds"),
					MountPath: pulumi.String("/var/run/secrets/cloudsql"),
					ReadOnly:  pulumi.Bool(true),
				},
			},
		}
	}).(v1.ContainerOutput)
}
