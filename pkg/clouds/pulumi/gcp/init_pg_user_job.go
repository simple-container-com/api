package gcp

import (
	"fmt"

	"github.com/samber/lo"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/clouds/pulumi/kubernetes"
	"github.com/simple-container-com/api/pkg/util"
)

const MaxInitSQLTimeSec = 30

type CloudsqlInstanceType string

const (
	PostgreSQL CloudsqlInstanceType = "POSTGRES"
	MySQL      CloudsqlInstanceType = "MYSQL"
)

type CloudsqlDbUser struct {
	Database string
	Username string
}

type InitDbUserJobArgs struct {
	User           CloudsqlDbUser
	RootPassword   string
	DBInstance     PostgresDBInstanceArgs
	CloudSQLProxy  *CloudSQLProxy
	KubeProvider   *sdkK8s.Provider
	DBInstanceType CloudsqlInstanceType
	Namespace      string
	Opts           []sdk.ResourceOption
}

type InitUserJob struct {
	Job sdk.Output
}

func NewInitDbUserJob(ctx *sdk.Context, stackName string, args InitDbUserJobArgs) (*InitUserJob, error) {
	// Sanitize stack name to comply with Kubernetes RFC 1123 requirements (no underscores)
	sanitizedStackName := kubernetes.SanitizeK8sName(stackName)
	jobName := util.TrimStringMiddle(fmt.Sprintf("%s-db-user-init", sanitizedStackName), 60, "-")
	jobCredsName := util.TrimStringMiddle(fmt.Sprintf("%s-creds", jobName), 60, "-")

	opts := args.Opts
	opts = append(opts, sdk.Provider(args.KubeProvider))

	// Secret creation
	jobCredsSecret, err := corev1.NewSecret(ctx, jobCredsName, &corev1.SecretArgs{
		Metadata: &v1.ObjectMetaArgs{
			Namespace: sdk.String(args.Namespace),
			Name:      sdk.String(jobCredsName),
		},
		StringData: sdk.StringMap{
			"PGPASSWORD": sdk.String(args.RootPassword),
			"MYSQL_PWD":  sdk.String(args.RootPassword),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	// Job Container creation
	jobContainer := sdk.All(args.User.Database, args.User.Username).ApplyT(func(all []interface{}) corev1.ContainerArgs {
		database := all[0].(string)
		username := all[1].(string)

		var initScript string
		if args.DBInstanceType == MySQL {
			initScript = `
                set -e;
                apk add --no-cache mysql-client; 
                sleep 20;
                # MySQL-specific logic here
            `
		} else {
			initScript = fmt.Sprintf(`
set -e;
apk add --no-cache postgresql-client; 
sleep 20;
psql -h localhost -U postgres -d %s -c 'GRANT pg_read_all_data TO "%s";';
psql -h localhost -U postgres -d %s -c 'GRANT pg_write_all_data TO "%s";';
            `, database, username, database, username)
		}

		return corev1.ContainerArgs{
			Command: sdk.StringArray{
				sdk.String("sh"),
				sdk.String("-c"),
				sdk.String(initScript),
			},
			EnvFrom: corev1.EnvFromSourceArray{
				&corev1.EnvFromSourceArgs{
					SecretRef: &corev1.SecretEnvSourceArgs{
						Name: jobCredsSecret.Metadata.Name().Elem(),
					},
				},
			},
			Image: sdk.String("alpine:latest"),
			Name:  sdk.String("job"),
		}
	})

	cloudsqlProxy := args.CloudSQLProxy
	kubeProvider := args.KubeProvider
	namespace := args.Namespace

	jobOut := sdk.All(cloudsqlProxy.SqlProxySecret.Metadata.Name(), jobContainer, cloudsqlProxy.ProxyContainer).ApplyT(func(args []any) (*batchv1.Job, error) {
		secretName := args[0].(*string)
		jobContainerArgs := args[1].(corev1.ContainerArgs)
		proxyContainerArgs := args[2].(corev1.ContainerArgs)

		// Job creation
		job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
			Metadata: &v1.ObjectMetaArgs{
				Name:      sdk.String(jobName),
				Namespace: sdk.String(namespace),
				Annotations: sdk.StringMap{
					"pulumi.com/patchForce": sdk.String("true"),
				},
			},
			Spec: &batchv1.JobSpecArgs{
				BackoffLimit: sdk.Int(5),
				Template: &corev1.PodTemplateSpecArgs{
					Spec: &corev1.PodSpecArgs{
						Containers:    corev1.ContainerArray{jobContainerArgs, proxyContainerArgs},
						RestartPolicy: sdk.String("Never"),
						Volumes: corev1.VolumeArray{
							&corev1.VolumeArgs{
								Name: sdk.String(lo.FromPtr(secretName)),
								Secret: &corev1.SecretVolumeSourceArgs{
									SecretName: sdk.StringPtrFromPtr(secretName),
								},
							},
						},
					},
				},
			},
		}, append(opts, sdk.Provider(kubeProvider))...)
		if err != nil {
			return nil, err
		}
		return job, nil
	})

	if err != nil {
		return nil, err
	}

	// Proxy Container creation
	return &InitUserJob{
		Job: jobOut,
	}, nil
}
